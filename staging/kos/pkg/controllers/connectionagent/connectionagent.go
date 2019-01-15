/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package connectionagent

import (
	"bytes"
	"fmt"
	gonet "net"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	k8sutilruntime "k8s.io/apimachinery/pkg/util/runtime"
	k8swait "k8s.io/apimachinery/pkg/util/wait"
	k8scache "k8s.io/client-go/tools/cache"
	k8sworkqueue "k8s.io/client-go/util/workqueue"

	netv1a1 "k8s.io/examples/staging/kos/pkg/apis/network/v1alpha1"
	kosclientset "k8s.io/examples/staging/kos/pkg/client/clientset/versioned"
	netvifc1a1 "k8s.io/examples/staging/kos/pkg/client/clientset/versioned/typed/network/v1alpha1"
	kosinformers "k8s.io/examples/staging/kos/pkg/client/informers/externalversions"
	kosinternalifcs "k8s.io/examples/staging/kos/pkg/client/informers/externalversions/internalinterfaces"
	kosinformersv1a1 "k8s.io/examples/staging/kos/pkg/client/informers/externalversions/network/v1alpha1"
	koslisterv1a1 "k8s.io/examples/staging/kos/pkg/client/listers/network/v1alpha1"
	kosctlrutils "k8s.io/examples/staging/kos/pkg/controllers/utils"
	netfabric "k8s.io/examples/staging/kos/pkg/networkfabric"
)

const (
	// Name of the indexer which computes the MAC address for a network
	// attachment. Used for syncing pre-existing interfaces at start-up.
	attMACIndexName = "attachmentMAC"

	// NetworkAttachments in network.example.com/v1alpha1
	// fields names. Used to build field selectors.
	attNodeFieldName   = "spec.node"
	attIPFieldName     = "status.ipv4"
	attHostIPFieldName = "status.hostIP"
	attVNIFieldName    = "status.addressVNI"

	// fields selector comparison operators.
	// Used to build fields selectors.
	equal    = "="
	notEqual = "!="

	// resync period for Informers caches. Set
	// to 0 because we don't want resyncs.
	resyncPeriod = 0

	// netFabricRetryPeriod is the time we wait before retrying when an
	// network fabric operation fails while handling pre-existing interfaces.
	netFabricRetryPeriod = time.Second
)

// vnState stores all the state needed for a Virtual Network for
// which there is at least one NetworkAttachment local to this node.
type vnState struct {
	// remoteAttsInformer is an informer on the NetworkAttachments that are
	// both: (1) in the Virtual Network the vnState represents, (2) not on
	// this node. It is stopped when the last local NetworkAttachment in the
	// Virtual Network associated with the vnState instance is deleted. To
	// stop it, remoteAttsInformerStopCh must be closed.
	remoteAttsInformer       k8scache.SharedIndexInformer
	remoteAttsInformerStopCh chan struct{}

	// remoteAttsLister is a lister on the NetworkAttachments that are
	// both: (1) in the Virtual Network the vnState represents, (2) not
	// on this node. Since a Virtual Network cannot span multiple k8s API
	// namespaces, it's a NamespaceLister.
	remoteAttsLister koslisterv1a1.NetworkAttachmentNamespaceLister

	// namespace is the namespace of the Virtual Network
	// associated with this vnState.
	namespace string

	// localAtts and remoteAtts store the names of the local and remote
	// NetworkAttachments in the Virtual Network the vnState represents,
	// respectively. localAtts is used to detect when the last local attachment
	// in the virtual network is deleted, so that remoteAttsInformer can be
	// stopped. remoteAtts is used to enqueue references to the remote
	// attachments in the Virtual Network when such Virtual Network becomes
	// irrelevant (deletion of last local attachment), so that the interfaces of
	// the remote attachments can be deleted.
	localAtts  map[string]struct{}
	remoteAtts map[string]struct{}
}

// ConnectionAgent represents a K8S controller which runs on every node of the
// cluster and eagerly maintains up-to-date the mapping between virtual IPs and
// physical IPs for every relevant NetworkAttachment. A NetworkAttachment is
// relevant to a connection agent if: (1) it runs on the same node as the
// connection agent, or (2) it's part of a Virtual Network where at least one
// NetworkAttachment for which (1) is true exists. To achieve its goal, a
// connection agent receives notifications about relevant NetworkAttachments
// from the K8s API server through Informers, and when necessary
// creates/updates/deletes Network Interfaces through a low-level network
// interface fabric. When a new Virtual Network becomes relevant for the
// connection agent because of the creation of the first attachment of that
// Virtual Network on the same node as the connection agent, a new informer on
// remote NetworkAttachments in that Virtual Network is created. Upon being
// notified of the creation of a local NetworkAttachment, the connection agent
// also updates the status of such attachment with its host IP and the name of
// the interface which was created.
type ConnectionAgent struct {
	localNodeName string
	hostIP        gonet.IP
	kcs           *kosclientset.Clientset
	netv1a1Ifc    netvifc1a1.NetworkV1alpha1Interface
	queue         k8sworkqueue.RateLimitingInterface
	workers       int
	netFabric     netfabric.Interface
	stopCh        <-chan struct{}

	// Informer and lister on NetworkAttachments on the same node as the
	// connection agent
	localAttsInformer k8scache.SharedIndexInformer
	localAttsLister   koslisterv1a1.NetworkAttachmentLister

	// Map from vni to vnState associated with that vni. Accessed only while
	// holding vniToVnStateMutex
	vniToVnStateMutex sync.RWMutex
	vniToVnState      map[uint32]*vnState

	// nsnToVNStateVNI maps local attachments namespaced names to the VNI of the
	// vnState they're stored in. Accessed only while holding nsnToVNStateVNIMutex.
	nsnToVNStateVNIMutex sync.RWMutex
	nsnToVNStateVNI      map[k8stypes.NamespacedName]uint32

	nsnToLocalIfcMutex sync.RWMutex
	nsnToLocalIfc      map[k8stypes.NamespacedName]netfabric.LocalNetIfc

	nsnToRemoteIfcMutex sync.RWMutex
	nsnToRemoteIfc      map[k8stypes.NamespacedName]netfabric.RemoteNetIfc

	// nsnToVNIs maps attachments (both local and remote) namespaced names
	// to set of vnis where the attachments have been seen. It is accessed by the
	// notification handlers for remote attachments, which add/remove the vni
	// with which they see the attachment upon creation/deletion of the attachment
	// respectively. When a worker processes an attachment reference, it reads
	// from nsnToVNIs the vnis with which the attachment has been seen. If there's
	// more than one vni, the current state of the attachment is ambiguous and
	// the worker stops the processing. The deletion notification handler for one
	// of the VNIs of the attachment will cause the reference to be requeued
	// and hopefully by the time it is dequeued again the ambiguity as for the
	// current attachment state has been resolved. Accessed only while holding
	// nsnToVNIsMutex.
	nsnToVNIsMutex sync.RWMutex
	nsnToVNIs      map[k8stypes.NamespacedName]map[uint32]struct{}
}

// NewConnectionAgent returns a deactivated instance of a ConnectionAgent (neither
// the workers goroutines nor any Informer have been started). Invoke Run to activate.
func NewConnectionAgent(localNodeName string,
	hostIP gonet.IP,
	kcs *kosclientset.Clientset,
	queue k8sworkqueue.RateLimitingInterface,
	workers int,
	netFabric netfabric.Interface) *ConnectionAgent {

	return &ConnectionAgent{
		localNodeName:   localNodeName,
		hostIP:          hostIP,
		kcs:             kcs,
		netv1a1Ifc:      kcs.NetworkV1alpha1(),
		queue:           queue,
		workers:         workers,
		netFabric:       netFabric,
		vniToVnState:    make(map[uint32]*vnState),
		nsnToVNStateVNI: make(map[k8stypes.NamespacedName]uint32),
		nsnToLocalIfc:   make(map[k8stypes.NamespacedName]netfabric.LocalNetIfc),
		nsnToRemoteIfc:  make(map[k8stypes.NamespacedName]netfabric.RemoteNetIfc),
		nsnToVNIs:       make(map[k8stypes.NamespacedName]map[uint32]struct{}),
	}
}

// Run activates the ConnectionAgent: the local attachments informer is started,
// pre-existing network interfaces on the node are synced, and the worker
// goroutines are started. Close stopCh to stop the ConnectionAgent.
func (ca *ConnectionAgent) Run(stopCh <-chan struct{}) error {
	defer k8sutilruntime.HandleCrash()
	defer ca.queue.ShutDown()

	ca.stopCh = stopCh
	ca.initLocalAttsInformerAndLister()
	go ca.localAttsInformer.Run(stopCh)
	glog.V(2).Infoln("local NetworkAttachments informer started")

	if err := ca.waitForLocalAttsCacheSync(stopCh); err != nil {
		return err
	}
	glog.V(2).Infoln("local NetworkAttachments cache synced")

	if err := ca.syncPreExistingIfcs(); err != nil {
		return err
	}
	glog.V(2).Infoln("pre-existing interfaces synced")

	for i := 0; i < ca.workers; i++ {
		go k8swait.Until(ca.processQueue, time.Second, stopCh)
	}
	glog.V(2).Infof("launched %d workers", ca.workers)

	<-stopCh
	return nil
}

func (ca *ConnectionAgent) initLocalAttsInformerAndLister() {
	localAttWithAnIPSelector := ca.localAttWithAnIPSelector()

	ca.localAttsInformer, ca.localAttsLister = v1a1AttsCustomInformerAndLister(ca.kcs,
		resyncPeriod,
		fromFieldsSelectorToTweakListOptionsFunc(localAttWithAnIPSelector))

	ca.localAttsInformer.AddIndexers(map[string]k8scache.IndexFunc{attMACIndexName: attachmentMACAddr})

	ca.localAttsInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    ca.onLocalAttAdded,
		UpdateFunc: ca.onLocalAttUpdated,
		DeleteFunc: ca.onLocalAttRemoved,
	})
}

func (ca *ConnectionAgent) onLocalAttAdded(obj interface{}) {
	att := obj.(*netv1a1.NetworkAttachment)
	glog.V(5).Infof("local NetworkAttachments cache: notified of addition of %#+v", att)
	ca.queue.Add(kosctlrutils.AttNSN(att))
}

func (ca *ConnectionAgent) onLocalAttUpdated(oldObj, newObj interface{}) {
	oldAtt := oldObj.(*netv1a1.NetworkAttachment)
	newAtt := newObj.(*netv1a1.NetworkAttachment)
	glog.V(5).Infof("local NetworkAttachments cache: notified of update from %#+v to %#+v",
		oldAtt,
		newAtt)
	ca.queue.Add(kosctlrutils.AttNSN(newAtt))
}

func (ca *ConnectionAgent) onLocalAttRemoved(obj interface{}) {
	peeledObj := kosctlrutils.Peel(obj)
	att := peeledObj.(*netv1a1.NetworkAttachment)
	glog.V(5).Infof("local NetworkAttachments cache: notified of removal of %#+v", att)
	ca.queue.Add(kosctlrutils.AttNSN(att))
}

func (ca *ConnectionAgent) waitForLocalAttsCacheSync(stopCh <-chan struct{}) error {
	if !k8scache.WaitForCacheSync(stopCh, ca.localAttsInformer.HasSynced) {
		return fmt.Errorf("caches failed to sync")
	}
	return nil
}

func (ca *ConnectionAgent) syncPreExistingIfcs() error {
	if err := ca.syncPreExistingLocalIfcs(); err != nil {
		return err
	}

	return ca.syncPreExistingRemoteIfcs()
}

func (ca *ConnectionAgent) syncPreExistingLocalIfcs() error {
	allPreExistingLocalIfcs, err := ca.netFabric.ListLocalIfcs()
	if err != nil {
		return fmt.Errorf("failed initial local network interfaces list: %s", err.Error())
	}

	for _, aPreExistingLocalIfc := range allPreExistingLocalIfcs {
		ifcMAC := aPreExistingLocalIfc.GuestMAC.String()
		ifcOwnerAtts, err := ca.localAttsInformer.GetIndexer().ByIndex(attMACIndexName, ifcMAC)
		if err != nil {
			return fmt.Errorf("indexing local network interface with MAC %s failed: %s",
				ifcMAC,
				err.Error())
		}

		if len(ifcOwnerAtts) == 1 {
			// If we're here there's a local attachment which should own the
			// interface because their MAC addresses match. Hence we add the
			// interface to the attachment state.
			ifcOwnerAtt := ifcOwnerAtts[0].(*netv1a1.NetworkAttachment)
			nsn := kosctlrutils.AttNSN(ifcOwnerAtt)
			oldIfc, oldIfcExists := ca.getLocalIfc(nsn)
			ca.assignLocalIfc(nsn, aPreExistingLocalIfc)
			glog.V(3).Infof("matched interface %#+v with local attachment %#+v", aPreExistingLocalIfc, ifcOwnerAtt)
			if oldIfcExists {
				aPreExistingLocalIfc = oldIfc
			} else {
				continue
			}
		}

		// If we're here the interface must be deleted, e.g. because it could
		// not be matched to an attachment, or because the attachment to which
		// it has already been matched has changed and was matched to a different
		// interface.
		for i, err := 1, ca.netFabric.DeleteLocalIfc(aPreExistingLocalIfc); err != nil; i++ {
			glog.V(3).Infof("deletion of orphan local interface %#+v failed: %s. Attempt nbr. %d",
				aPreExistingLocalIfc,
				err.Error(),
				i)
			time.Sleep(netFabricRetryPeriod)
		}
		glog.V(3).Infof("deleted orphan local interface %#+v", aPreExistingLocalIfc)
	}

	return nil
}

func (ca *ConnectionAgent) syncPreExistingRemoteIfcs() error {
	// Start all remote attachments caches because we need to look up remote
	// attachments to decide which interfaces to keep and which to delete.
	allLocalAtts, err := ca.localAttsLister.List(k8slabels.Everything())
	if err != nil {
		return fmt.Errorf("failed initial local attachments list: %s", err.Error())
	}
	for _, aLocalAtt := range allLocalAtts {
		nsn, attVNI := kosctlrutils.AttNSN(aLocalAtt), aLocalAtt.Status.AddressVNI
		ca.updateVNStateForExistingAtt(nsn, true, attVNI)
	}

	// Read all remote ifcs, for each interface find the attachment with the same
	// MAC in the cache for the remote attachments with the same VNI as the interface.
	// If either the attachment or the cache are not found, delete the interface,
	// bind it to the attachment otherwise.
	allPreExistingRemoteIfcs, err := ca.netFabric.ListRemoteIfcs()
	if err != nil {
		return fmt.Errorf("failed initial remote network interfaces list: %s", err.Error())
	}
	for _, aPreExistingRemoteIfc := range allPreExistingRemoteIfcs {
		var ifcOwnerAtts []interface{}
		ifcMAC, ifcVNI := aPreExistingRemoteIfc.GuestMAC.String(), aPreExistingRemoteIfc.VNI
		remoteAttsInformer, remoteAttsInformerStopCh := ca.getRemoteAttsInformerForVNI(ifcVNI)
		if remoteAttsInformer != nil {
			if !remoteAttsInformer.HasSynced() &&
				!k8scache.WaitForCacheSync(remoteAttsInformerStopCh, remoteAttsInformer.HasSynced) {
				return fmt.Errorf("failed to sync cache of remote attachments for VNI %d", ifcVNI)
			}
			ifcOwnerAtts, err = remoteAttsInformer.GetIndexer().ByIndex(attMACIndexName, ifcMAC)
		}

		if len(ifcOwnerAtts) == 1 {
			// If we're here a remote attachment owning the interface has been found
			ifcOwnerAtt := ifcOwnerAtts[0].(*netv1a1.NetworkAttachment)
			nsn := kosctlrutils.AttNSN(ifcOwnerAtt)
			oldRemoteIfc, oldRemoteIfcExists := ca.getRemoteIfc(nsn)
			ca.assignRemoteIfc(nsn, aPreExistingRemoteIfc)
			glog.V(3).Infof("matched interface %#+v with remote attachment %#+v",
				aPreExistingRemoteIfc,
				ifcOwnerAtt)
			if oldRemoteIfcExists {
				aPreExistingRemoteIfc = oldRemoteIfc
			} else {
				if oldLocalIfc, oldLocalIfcExists := ca.getLocalIfc(nsn); oldLocalIfcExists {
					for i, err := 1, ca.netFabric.DeleteLocalIfc(oldLocalIfc); err != nil; i++ {
						glog.V(3).Infof("deletion of orphan local interface %#+v failed: %s. Attempt nbr. %d",
							oldLocalIfc,
							err.Error(),
							i)
						time.Sleep(netFabricRetryPeriod)
					}
					glog.V(3).Infof("deleted orphan local interface %#+v", oldLocalIfc)
				}
				continue
			}
		}

		// If we're here either there's no remote attachments cache associated with
		// the interface vni (because there are no local attachments with that vni),
		// or no remote attachment owning the interface was found, or the attachment
		// owning the interface already has one. For all such cases we need to delete
		// the interface.
		for i, err := 1, ca.netFabric.DeleteRemoteIfc(aPreExistingRemoteIfc); err != nil; i++ {
			glog.V(3).Infof("deletion of orphan remote interface %#+v failed: %s. Attempt nbr. %d",
				aPreExistingRemoteIfc,
				err.Error(),
				i)
			time.Sleep(netFabricRetryPeriod)
		}
		glog.V(3).Infof("deleted orphan remote interface %#+v", aPreExistingRemoteIfc)
	}

	return nil
}

func (ca *ConnectionAgent) processQueue() {
	for {
		item, stop := ca.queue.Get()
		if stop {
			return
		}
		attNSN := item.(k8stypes.NamespacedName)
		ca.processQueueItem(attNSN)
	}
}

func (ca *ConnectionAgent) processQueueItem(attNSN k8stypes.NamespacedName) {
	defer ca.queue.Done(attNSN)
	err := ca.processNetworkAttachment(attNSN)
	requeues := ca.queue.NumRequeues(attNSN)
	if err != nil {
		// If we're here there's been an error: either the attachment current state was
		// ambiguous (e.g. more than one vni), or there's been a problem while processing
		// it (e.g. Interface creation failed). We requeue the attachment reference so that
		// it can be processed again and hopefully next time there will be no errors.
		glog.Warningf("failed processing NetworkAttachment %s, requeuing (%d earlier requeues): %s",
			attNSN,
			requeues,
			err.Error())
		ca.queue.AddRateLimited(attNSN)
		return
	}
	glog.V(4).Infof("finished NetworkAttachment %s with %d requeues", attNSN, requeues)
	ca.queue.Forget(attNSN)
}

func (ca *ConnectionAgent) processNetworkAttachment(attNSN k8stypes.NamespacedName) error {
	att, deleted := ca.getAttachment(attNSN)
	if att != nil {
		// If we are here the attachment exists and it's current state is univocal
		return ca.processExistingAtt(att)
	} else if deleted {
		// If we are here the attachment has been deleted
		return ca.processDeletedAtt(attNSN)
	}
	return nil
}

// getAttachment attempts to determine the univocal version of the NetworkAttachment
// with namespaced name attNSN. If it succeeds it returns the attachment if it is
// found in an Informer cache or a boolean flag set to true if it could not be found
// in any cache (e.g. because it has been deleted). If the current attachment
// version cannot be determined without ambiguity, the attachment return value is nil,
// and the deleted flag is set to false. An attachment is considered amibguous if
// it either has been seen with more than one vni in a remote attachments cache,
// or if it is found both in the local attachments cache and a remote attachments
// cache.
func (ca *ConnectionAgent) getAttachment(attNSN k8stypes.NamespacedName) (*netv1a1.NetworkAttachment, bool) {
	// Retrieve the number of VN(I)s where the attachment could be as a remote
	// attachment, or, if it could be only in one VN(I), return that VNI.
	vni, nbrOfVNIs := ca.getAttSeenInVNI(attNSN)
	if nbrOfVNIs > 1 {
		// If the attachment could be a remote one in more than one VNI, we
		// return immediately. When a deletion notification handler removes the
		// VNI with which it's seeing the attachment the attachment state will be
		// "less ambiguous" (one less potential VNI) and a reference will be enqueued
		// again triggering reconsideration of the attachment.
		glog.V(4).Infof("attachment %s has inconsistent state, found in %d VN(I)s",
			attNSN,
			nbrOfVNIs)
		return nil, false
	}

	// If the attachment has been seen in exactly one VNI lookup it up in
	// the remote attachments cache for the VNI with which it's been seen
	var (
		attAsRemote          *netv1a1.NetworkAttachment
		remAttCacheLookupErr error
	)
	if nbrOfVNIs == 1 {
		remoteAttsLister := ca.getRemoteAttListerForVNI(vni)
		if remoteAttsLister != nil {
			attAsRemote, remAttCacheLookupErr = remoteAttsLister.Get(attNSN.Name)
		}
	}

	// Lookup the attachment in the local attachments cache
	attAsLocal, localAttCacheLookupErr := ca.localAttsLister.NetworkAttachments(attNSN.Namespace).Get(attNSN.Name)

	switch {
	case (remAttCacheLookupErr != nil && !k8serrors.IsNotFound(remAttCacheLookupErr)) ||
		(localAttCacheLookupErr != nil && !k8serrors.IsNotFound(localAttCacheLookupErr)):
		// If we're here at least one of the two lookups failed. This should
		// never happen. No point in retrying.
		glog.V(1).Infof("attempt to retrieve attachment %s with lister failed: %s. This should never happen, hence a reference to %s will not be requeued",
			attNSN,
			aggregateErrors("\n\t", remAttCacheLookupErr, localAttCacheLookupErr).Error(),
			attNSN)
	case attAsLocal != nil && attAsRemote != nil:
		// If we're here the attachment has been found in both caches, hence it's
		// state is ambiguous. It will be deleted by one of the caches soon, and
		// this will cause a reference to be enqueued, so it will be processed
		// again when the ambiguity has been resolved (assuming it has not been
		// seen with other VNIs meanwhile).
		glog.V(4).Infof("att %s has inconsistent state: found both in local atts cache and remote atts cache for VNI %d",
			attNSN,
			vni)
	case attAsLocal != nil && attAsRemote == nil:
		// If we're here the attachment was found only in the local cache:
		// that's the univocal version of the attachment
		return attAsLocal, false
	case attAsLocal == nil && attAsRemote != nil:
		// If we're here the attachment was found only in the remote attachments
		// cache for its vni: that's the univocal version of the attachment
		return attAsRemote, false
	}
	// If we're here neither lookup could find the attachment: we assume the
	// attachment has been deleted by both caches and is therefore no longer
	// relevant to the connection agent
	return nil, true
}

func (ca *ConnectionAgent) processExistingAtt(att *netv1a1.NetworkAttachment) error {
	attNSN, attVNI := kosctlrutils.AttNSN(att), att.Status.AddressVNI
	attNode := att.Spec.Node

	// Update the vnState associated with the attachment. This typically involves
	// adding the attachment to the vnState associated to its vni (and initializing
	// that vnState if the attachment is the first local one with its vni), but
	// could also entail removing the attachment from the vnState associated with
	// its old vni if the vni has changed.
	vnState, noVnStateFoundForRemoteAtt := ca.updateVNState(attVNI, attNSN, attNode)
	if vnState != nil {
		// If we're here att is currently remote but was previously the last local
		// attachment in its vni. Thus, we act as if the last local attachment
		// in the vn was deleted
		ca.clearVNResources(vnState, attNSN.Name, attVNI)
		return nil
	}
	if noVnStateFoundForRemoteAtt {
		// If we're here att is remote but its vnState has been removed because
		// of the deletion of the last local attachment in its virtual network
		// between the lookup in the remote attachments cache and the attempt to
		// set the attachment name into its vnState, hence we treat it as a deleted
		// attachment.
		ca.removeSeenInVNI(attNSN, attVNI)
		return ca.processDeletedAtt(attNSN)
	}

	// Create or update the interface associated with the attachment.
	var attHostIP gonet.IP
	if attNode == ca.localNodeName {
		attHostIP = ca.hostIP
	} else {
		attHostIP = gonet.ParseIP(att.Status.HostIP)
	}
	attGuestIP := gonet.ParseIP(att.Status.IPv4)
	newLocalIfcName, err := ca.createOrUpdateIfc(attGuestIP,
		attHostIP,
		attVNI,
		attNSN)
	if err != nil {
		return err
	}

	// If the attachment is local, update its status with the local host IP and
	// the name of the interface which was created (if it has changed).
	localHostIPStr := ca.hostIP.String()
	if attNode == ca.localNodeName &&
		(att.Status.HostIP != localHostIPStr || (newLocalIfcName != "" && newLocalIfcName != att.Status.IfcName)) {

		updatedAtt, err := ca.setAttStatus(att, newLocalIfcName)
		if err != nil {
			return err
		}
		glog.V(3).Infof("updated att %s status with hostIP: %s, ifcName: %s",
			attNSN,
			updatedAtt.Status.HostIP,
			updatedAtt.Status.IfcName)
	}

	return nil
}

func (ca *ConnectionAgent) processDeletedAtt(attNSN k8stypes.NamespacedName) error {
	vnStateVNI, vnStateVNIFound := ca.getVNStateVNI(attNSN)
	if vnStateVNIFound {
		ca.updateVNStateAfterAttDeparture(attNSN.Name, vnStateVNI)
		ca.unsetVNStateVNI(attNSN)
	}

	localIfc, attHasLocalIfc := ca.getLocalIfc(attNSN)
	if attHasLocalIfc {
		if err := ca.netFabric.DeleteLocalIfc(localIfc); err != nil {
			return err
		}
		ca.unsetLocalIfc(attNSN)
		return nil
	}

	remoteIfc, attHasRemoteIfc := ca.getRemoteIfc(attNSN)
	if attHasRemoteIfc {
		if err := ca.netFabric.DeleteRemoteIfc(remoteIfc); err != nil {
			return err
		}
		ca.unsetRemoteIfc(attNSN)
	}

	return nil
}

func (ca *ConnectionAgent) updateVNState(attNewVNI uint32,
	attNSN k8stypes.NamespacedName,
	attNode string) (*vnState, bool) {

	attOldVNI, oldVNIFound := ca.getVNStateVNI(attNSN)
	if oldVNIFound && attOldVNI != attNewVNI {
		// if we're here the attachment vni changed since the last time it
		// was processed, hence we update the vnState associated with the
		// old value of the vni to reflect the attachment departure.
		ca.updateVNStateAfterAttDeparture(attNSN.Name, attOldVNI)
		ca.unsetVNStateVNI(attNSN)
	}

	return ca.updateVNStateForExistingAtt(attNSN, attNode == ca.localNodeName, attNewVNI)
}

// updateVNStateForExistingAtt adds the attachment to the vnState associated with
// its vni. If the attachment is local and is the first one for its vni, the
// associated vnState is initialized (this entails starting the remote attachments
// informer). If the attachment was the last local attachment in its vnState and
// has become remote, the vnState for its vni is cleared (it's removed from the
// map storing the vnStates) and returned, so that the caller can perform a clean
// up of the resources associated with the vnState (remote attachments informer
// is stopped and references to the remote attachments are enqueued). If the
// attachment is remote and its vnState cannot be found (because the last local
// attachment in the same Virtual Network has been deleted) noVnStateFoundForRemoteAtt
// is set to false so that the caller knows and can react appropriately.
func (ca *ConnectionAgent) updateVNStateForExistingAtt(attNSN k8stypes.NamespacedName,
	attIsLocal bool,
	vni uint32) (vnStateRet *vnState, noVnStateFoundForRemoteAtt bool) {

	attName := attNSN.Name
	firstLocalAttInVN := false

	ca.vniToVnStateMutex.Lock()
	defer func() {
		ca.vniToVnStateMutex.Unlock()
		if vnStateRet == nil && !noVnStateFoundForRemoteAtt {
			ca.setVNStateVNI(attNSN, vni)
		} else {
			ca.unsetVNStateVNI(attNSN)
		}
		if firstLocalAttInVN {
			glog.V(2).Infof("VN with ID %d became relevant: an Informer has been started", vni)
		}
	}()

	vnState := ca.vniToVnState[vni]
	if attIsLocal {
		// If we're here the attachment is local. If the vnState for the
		// attachment vni is missing it means that the attachment is the first
		// local one for its vni, hence we initialize the vnState (this entails
		// starting the remote attachments informer). We also add the attachment
		// name to the local attachments in the virtual network and remove the
		// attachment name from the remote attachments: this is needed in case
		// we're here because of an update which did not change the vni but made
		// the attachment transition from remote to local.
		if vnState == nil {
			vnState = ca.initVNState(vni, attNSN.Namespace)
			ca.vniToVnState[vni] = vnState
			firstLocalAttInVN = true
		}
		delete(vnState.remoteAtts, attName)
		vnState.localAtts[attName] = struct{}{}
	} else {
		// If we're here the attachment is remote. If the vnState for the
		// attachment vni is not missing (because the last local attachment with
		// the same vni has been deleted), we add the attachment name to the
		// remote attachments in the vnState. Then we remove the attachment name from
		// the local attachments: this is needed in case we're here because of
		// an update which did not change the vni but made the attachment
		// transition from local to remote. After doing this, we check whether
		// the local attachment we've removed was the last one for its vni. If
		// that's the case (len(state.localAtts) == 0), the vni is no longer
		// relevant to the connection agent, hence we return the vnState after
		// removing it from the map storing all the vnStates so that the caller
		// can perform the necessary clean up. If the vnState is missing, we
		// set the return flag noVnStateFoundForRemoteAtt to false so that the
		// caller knows that the remote attachment it is processing must not get an
		// interface (this is needed because a reference to such attachment is
		// not necessarily already in the list of remote attachments in the vn, hence
		// it's not granted that such a reference has been enqueued to delete
		// the attachment).
		if vnState != nil {
			vnState.remoteAtts[attName] = struct{}{}
			delete(vnState.localAtts, attName)
			if len(vnState.localAtts) == 0 {
				delete(ca.vniToVnState, vni)
				vnStateRet = vnState
			}
		} else {
			noVnStateFoundForRemoteAtt = true
		}
	}

	return
}

func (ca *ConnectionAgent) updateVNStateAfterAttDeparture(attName string, vni uint32) {
	vnState := ca.removeAttFromVNState(attName, vni)
	if vnState == nil {
		return
	}
	// If we're here attName was the last local attachment in the virtual network
	// with id vni. Hence we stop the remote attachments informer and enqueue
	// references to remote attachments in that virtual network, so that their
	// interfaces can be deleted.
	ca.clearVNResources(vnState, attName, vni)
}

func (ca *ConnectionAgent) createOrUpdateIfc(attGuestIP, attHostIP gonet.IP,
	attVNI uint32,
	attNSN k8stypes.NamespacedName) (string, error) {

	attMAC := generateMACAddr(attVNI, attGuestIP)
	oldLocalIfc, attHasLocalIfc := ca.getLocalIfc(attNSN)
	oldRemoteIfc, attHasRemoteIfc := ca.getRemoteIfc(attNSN)
	newIfcNeedsToBeCreated := (!attHasLocalIfc && !attHasRemoteIfc) ||
		(attHasLocalIfc && ifcNeedsUpdate(oldLocalIfc.HostIP, attHostIP, oldLocalIfc.GuestMAC, attMAC)) ||
		(attHasRemoteIfc && ifcNeedsUpdate(oldRemoteIfc.HostIP, attHostIP, oldRemoteIfc.GuestMAC, attMAC))

	var newLocalIfcName string
	if newIfcNeedsToBeCreated {
		if attHasLocalIfc {
			if err := ca.netFabric.DeleteLocalIfc(oldLocalIfc); err != nil {
				return "", fmt.Errorf("update of network interface of attachment %s failed, old local interface %#+v could not be deleted: %s",
					attNSN,
					oldLocalIfc,
					err.Error())
			}
			ca.unsetLocalIfc(attNSN)
		} else if attHasRemoteIfc {
			if err := ca.netFabric.DeleteRemoteIfc(oldRemoteIfc); err != nil {
				return "", fmt.Errorf("update of network interface of attachment %s failed, old remote interface %#+v could not be deleted: %s",
					attNSN,
					oldRemoteIfc,
					err.Error())
			}
			ca.unsetRemoteIfc(attNSN)
		}

		if attHostIP.Equal(ca.hostIP) {
			newLocalIfc := netfabric.LocalNetIfc{
				Name:     generateIfcName(attMAC),
				VNI:      attVNI,
				GuestMAC: attMAC,
				HostIP:   attHostIP,
			}
			if err := ca.netFabric.CreateLocalIfc(newLocalIfc); err != nil {
				return "", fmt.Errorf("creation of local network interface of attachment %s failed, interface %#+v could not be created: %s",
					attNSN,
					newLocalIfc,
					err.Error())
			}
			ca.assignLocalIfc(attNSN, newLocalIfc)
			newLocalIfcName = newLocalIfc.Name
		} else {
			newRemoteIfc := netfabric.RemoteNetIfc{
				VNI:      attVNI,
				GuestMAC: attMAC,
				HostIP:   attHostIP,
			}
			if err := ca.netFabric.CreateRemoteIfc(newRemoteIfc); err != nil {
				return "", fmt.Errorf("creation of remote network interface of attachment %s failed, interface %#+v could not be created: %s",
					attNSN,
					newRemoteIfc,
					err.Error())
			}
			ca.assignRemoteIfc(attNSN, newRemoteIfc)
		}
	}

	return newLocalIfcName, nil
}

func (ca *ConnectionAgent) setAttStatus(att *netv1a1.NetworkAttachment,
	ifcName string) (*netv1a1.NetworkAttachment, error) {

	att2 := att.DeepCopy()
	att2.Status.HostIP = ca.hostIP.String()
	att2.Status.IfcName = ifcName
	updatedAtt, err := ca.netv1a1Ifc.NetworkAttachments(att2.Namespace).Update(att2)
	return updatedAtt, err
}

// removeAttFromVNState removes attName from the vnState associated with vni, both
// for local and remote attachments. If attName is the last local attachment in
// the vnState, vnState is returned, so that the caller can perform additional
// clean up (e.g. stopping the remote attachments informer).
func (ca *ConnectionAgent) removeAttFromVNState(attName string, vni uint32) *vnState {
	ca.vniToVnStateMutex.Lock()
	defer ca.vniToVnStateMutex.Unlock()
	vnState := ca.vniToVnState[vni]
	if vnState != nil {
		delete(vnState.localAtts, attName)
		if len(vnState.localAtts) == 0 {
			delete(ca.vniToVnState, vni)
			return vnState
		}
		delete(vnState.remoteAtts, attName)
	}
	return nil
}

// clearVNResources stops the informer on remote attachments on the virtual
// network and enqueues references to such attachments so that their interfaces
// can be deleted.
func (ca *ConnectionAgent) clearVNResources(vnState *vnState, lastAttName string, vni uint32) {
	close(vnState.remoteAttsInformerStopCh)
	glog.V(2).Infof("networkAttachment %s/%s was the last local with vni %d: remote attachments informer was stopped",
		vnState.namespace,
		lastAttName,
		vni)

	for aRemoteAttName := range vnState.remoteAtts {
		aRemoteAttNSN := k8stypes.NamespacedName{
			Namespace: vnState.namespace,
			Name:      aRemoteAttName,
		}
		ca.removeSeenInVNI(aRemoteAttNSN, vni)
		ca.queue.Add(aRemoteAttNSN)
	}
}

func (ca *ConnectionAgent) initVNState(vni uint32, namespace string) *vnState {
	remoteAttsInformer, remoteAttsLister := v1a1AttsCustomNamespaceInformerAndLister(ca.kcs,
		resyncPeriod,
		namespace,
		fromFieldsSelectorToTweakListOptionsFunc(ca.remoteAttInVNWithVirtualIPHostIPSelector(vni)))

	remoteAttsInformer.AddIndexers(map[string]k8scache.IndexFunc{attMACIndexName: attachmentMACAddr})

	remoteAttsInformer.AddEventHandler(k8scache.ResourceEventHandlerFuncs{
		AddFunc:    ca.onRemoteAttAdded,
		UpdateFunc: ca.onRemoteAttUpdated,
		DeleteFunc: ca.onRemoteAttRemoved,
	})

	remoteAttsInformerStopCh := make(chan struct{})
	go remoteAttsInformer.Run(aggregateTwoStopChannels(ca.stopCh, remoteAttsInformerStopCh))

	return &vnState{
		remoteAttsInformer:       remoteAttsInformer,
		remoteAttsInformerStopCh: remoteAttsInformerStopCh,
		remoteAttsLister:         remoteAttsLister,
		namespace:                namespace,
		localAtts:                make(map[string]struct{}),
		remoteAtts:               make(map[string]struct{}),
	}
}

func (ca *ConnectionAgent) onRemoteAttAdded(obj interface{}) {
	att := obj.(*netv1a1.NetworkAttachment)
	glog.V(5).Infof("remote NetworkAttachments cache for VNI %d: notified of addition of %#+v",
		att.Status.AddressVNI,
		att)
	attNSN := kosctlrutils.AttNSN(att)
	ca.addVNI(attNSN, att.Status.AddressVNI)
	ca.queue.Add(attNSN)
}

func (ca *ConnectionAgent) onRemoteAttUpdated(oldObj, newObj interface{}) {
	oldAtt := oldObj.(*netv1a1.NetworkAttachment)
	newAtt := newObj.(*netv1a1.NetworkAttachment)
	glog.V(5).Infof("remote NetworkAttachments cache for VNI %d: notified of update from %#+v to %#+v",
		newAtt.Status.AddressVNI,
		oldAtt,
		newAtt)
	ca.queue.Add(kosctlrutils.AttNSN(newAtt))
}

func (ca *ConnectionAgent) onRemoteAttRemoved(obj interface{}) {
	peeledObj := kosctlrutils.Peel(obj)
	att := peeledObj.(*netv1a1.NetworkAttachment)
	glog.V(5).Infof("remote NetworkAttachments cache for VNI %d: notified of deletion of %#+v",
		att.Status.AddressVNI,
		att)
	attNSN := kosctlrutils.AttNSN(att)
	ca.removeSeenInVNI(attNSN, att.Status.AddressVNI)
	ca.queue.Add(attNSN)
}

func (ca *ConnectionAgent) getLocalIfc(nsn k8stypes.NamespacedName) (ifc netfabric.LocalNetIfc, ifcFound bool) {
	ca.nsnToLocalIfcMutex.RLock()
	defer ca.nsnToLocalIfcMutex.RUnlock()
	ifc, ifcFound = ca.nsnToLocalIfc[nsn]
	return
}

func (ca *ConnectionAgent) assignLocalIfc(nsn k8stypes.NamespacedName, ifc netfabric.LocalNetIfc) {
	ca.nsnToLocalIfcMutex.Lock()
	defer ca.nsnToLocalIfcMutex.Unlock()
	ca.nsnToLocalIfc[nsn] = ifc
}

func (ca *ConnectionAgent) getRemoteIfc(nsn k8stypes.NamespacedName) (ifc netfabric.RemoteNetIfc, ifcFound bool) {
	ca.nsnToRemoteIfcMutex.RLock()
	defer ca.nsnToRemoteIfcMutex.RUnlock()
	ifc, ifcFound = ca.nsnToRemoteIfc[nsn]
	return
}

func (ca *ConnectionAgent) assignRemoteIfc(nsn k8stypes.NamespacedName, ifc netfabric.RemoteNetIfc) {
	ca.nsnToRemoteIfcMutex.Lock()
	defer ca.nsnToRemoteIfcMutex.Unlock()
	ca.nsnToRemoteIfc[nsn] = ifc
}

func (ca *ConnectionAgent) getVNStateVNI(nsn k8stypes.NamespacedName) (vni uint32, vniFound bool) {
	ca.nsnToVNStateVNIMutex.RLock()
	defer ca.nsnToVNStateVNIMutex.RUnlock()
	vni, vniFound = ca.nsnToVNStateVNI[nsn]
	return
}

func (ca *ConnectionAgent) unsetVNStateVNI(nsn k8stypes.NamespacedName) {
	ca.nsnToVNStateVNIMutex.Lock()
	defer ca.nsnToVNStateVNIMutex.Unlock()
	delete(ca.nsnToVNStateVNI, nsn)
}

func (ca *ConnectionAgent) setVNStateVNI(nsn k8stypes.NamespacedName, vni uint32) {
	ca.nsnToVNStateVNIMutex.Lock()
	defer ca.nsnToVNStateVNIMutex.Unlock()
	ca.nsnToVNStateVNI[nsn] = vni
}

func (ca *ConnectionAgent) unsetLocalIfc(nsn k8stypes.NamespacedName) {
	ca.nsnToLocalIfcMutex.Lock()
	defer ca.nsnToLocalIfcMutex.Unlock()
	delete(ca.nsnToLocalIfc, nsn)
}

func (ca *ConnectionAgent) unsetRemoteIfc(nsn k8stypes.NamespacedName) {
	ca.nsnToRemoteIfcMutex.Lock()
	defer ca.nsnToRemoteIfcMutex.Unlock()
	delete(ca.nsnToRemoteIfc, nsn)
}

func (ca *ConnectionAgent) addVNI(nsn k8stypes.NamespacedName, vni uint32) {
	ca.nsnToVNIsMutex.Lock()
	defer ca.nsnToVNIsMutex.Unlock()
	attVNIs := ca.nsnToVNIs[nsn]
	if attVNIs == nil {
		attVNIs = make(map[uint32]struct{})
		ca.nsnToVNIs[nsn] = attVNIs
	}
	attVNIs[vni] = struct{}{}
}

func (ca *ConnectionAgent) removeSeenInVNI(nsn k8stypes.NamespacedName, vni uint32) {
	ca.nsnToVNIsMutex.Lock()
	defer ca.nsnToVNIsMutex.Unlock()
	attVNIs := ca.nsnToVNIs[nsn]
	if attVNIs == nil {
		return
	}
	delete(attVNIs, vni)
	if len(attVNIs) == 0 {
		delete(ca.nsnToVNIs, nsn)
	}
}

func (ca *ConnectionAgent) getAttSeenInVNI(nsn k8stypes.NamespacedName) (onlyVNI uint32, nbrOfVNIs int) {
	ca.nsnToVNIsMutex.RLock()
	defer ca.nsnToVNIsMutex.RUnlock()
	attVNIs := ca.nsnToVNIs[nsn]
	nbrOfVNIs = len(attVNIs)
	if nbrOfVNIs == 1 {
		for onlyVNI = range attVNIs {
		}
	}
	return
}

func (ca *ConnectionAgent) getRemoteAttListerForVNI(vni uint32) koslisterv1a1.NetworkAttachmentNamespaceLister {
	ca.vniToVnStateMutex.RLock()
	defer ca.vniToVnStateMutex.RUnlock()
	vnState := ca.vniToVnState[vni]
	if vnState == nil {
		return nil
	}
	return vnState.remoteAttsLister
}

// getRemoteAttsIndexerForVNI accesses the map with all the vnStates but it's not
// thread-safe because it is meant to be used only at start-up, when there's only
// one goroutine running.
func (ca *ConnectionAgent) getRemoteAttsInformerForVNI(vni uint32) (k8scache.SharedIndexInformer, chan struct{}) {
	vnState := ca.vniToVnState[vni]
	if vnState == nil {
		return nil, nil
	}
	return vnState.remoteAttsInformer, vnState.remoteAttsInformerStopCh
}

// Return a string representing a field selector that matches NetworkAttachments
// that run on the local node and have a virtual IP.
func (ca *ConnectionAgent) localAttWithAnIPSelector() string {
	// localAttSelector expresses the constraint that the NetworkAttachment runs
	// on this node.
	localAttSelector := attNodeFieldName + equal + ca.localNodeName

	// Express the constraint that the NetworkAttachment has a virtual IP by
	// saying that the field containig the virtual IP must not be equal to the
	// empty string.
	attWithAnIPSelector := attIPFieldName + notEqual

	// Build a selector which is a logical AND between
	// attWithAnIPSelectorString and localAttSelectorString.
	allSelectors := []string{localAttSelector, attWithAnIPSelector}
	return strings.Join(allSelectors, ",")
}

// Return a string representing a field selector that matches NetworkAttachments
// that run on a remote node on the Virtual Network identified by the given VNI
// and have a virtual IP and the host IP field set.
func (ca *ConnectionAgent) remoteAttInVNWithVirtualIPHostIPSelector(vni uint32) string {
	// remoteAttSelector expresses the constraint that the NetworkAttachment
	// runs on a remote node.
	remoteAttSelector := attNodeFieldName + notEqual + ca.localNodeName

	// hostIPIsNotLocalSelector expresses the constraint that the NetworkAttachment
	// status.hostIP is not equal to that of the current node. Without this selector,
	// an update to the spec.Node field of a NetworkAttachment could lead to a
	// creation notification for the attachment in a remote attachments cache,
	// even if the attachment still has the host IP of the current node
	// (status.hostIP is set with an update by the connection agent on the
	// node of the attachment). This could result in the creation of a remote
	// interface with the host IP of the local node.
	hostIPIsNotLocalSelector := attHostIPFieldName + notEqual + ca.hostIP.String()

	// attWithAnIPSelector and attWithHostIPSelector express the constraints that
	// the NetworkAttachment has the fields storing virtual IP and host IP set,
	// by saying that such fields must not be equal to the empty string.
	attWithAnIPSelector := attIPFieldName + notEqual
	attWithHostIPSelector := attHostIPFieldName + notEqual

	// attInSpecificVNSelector expresses the constraint that the NetworkAttachment
	// is in the Virtual Network identified by vni.
	attInSpecificVNSelector := attVNIFieldName + equal + fmt.Sprint(vni)

	// Build and return a selector which is a logical AND between all the selectors
	// defined above.
	allSelectors := []string{remoteAttSelector,
		hostIPIsNotLocalSelector,
		attWithAnIPSelector,
		attWithHostIPSelector,
		attInSpecificVNSelector}
	return strings.Join(allSelectors, ",")
}

func fromFieldsSelectorToTweakListOptionsFunc(customFieldSelector string) kosinternalifcs.TweakListOptionsFunc {
	return func(options *k8smetav1.ListOptions) {
		optionsFieldSelector := options.FieldSelector
		allSelectors := make([]string, 0, 2)
		if strings.Trim(optionsFieldSelector, " ") != "" {
			allSelectors = append(allSelectors, optionsFieldSelector)
		}
		allSelectors = append(allSelectors, customFieldSelector)
		options.FieldSelector = strings.Join(allSelectors, ",")
	}
}

func v1a1AttsCustomInformerAndLister(kcs *kosclientset.Clientset,
	resyncPeriod time.Duration,
	tweakListOptionsFunc kosinternalifcs.TweakListOptionsFunc) (k8scache.SharedIndexInformer, koslisterv1a1.NetworkAttachmentLister) {

	attv1a1Informer := createAttsv1a1Informer(kcs,
		resyncPeriod,
		k8smetav1.NamespaceAll,
		tweakListOptionsFunc)
	return attv1a1Informer.Informer(), attv1a1Informer.Lister()
}

func v1a1AttsCustomNamespaceInformerAndLister(kcs *kosclientset.Clientset,
	resyncPeriod time.Duration,
	namespace string,
	tweakListOptionsFunc kosinternalifcs.TweakListOptionsFunc) (k8scache.SharedIndexInformer, koslisterv1a1.NetworkAttachmentNamespaceLister) {

	attv1a1Informer := createAttsv1a1Informer(kcs,
		resyncPeriod,
		namespace,
		tweakListOptionsFunc)
	return attv1a1Informer.Informer(), attv1a1Informer.Lister().NetworkAttachments(namespace)
}

func createAttsv1a1Informer(kcs *kosclientset.Clientset,
	resyncPeriod time.Duration,
	namespace string,
	tweakListOptionsFunc kosinternalifcs.TweakListOptionsFunc) kosinformersv1a1.NetworkAttachmentInformer {

	localAttsInformerFactory := kosinformers.NewFilteredSharedInformerFactory(kcs,
		resyncPeriod,
		namespace,
		tweakListOptionsFunc)
	netv1a1Ifc := localAttsInformerFactory.Network().V1alpha1()
	return netv1a1Ifc.NetworkAttachments()
}

// attachmentMACAddr is an Index function that computes the MAC address of a
// NetworkAttachment. Used to sync pre-existing interfaces with attachments at
// start up.
func attachmentMACAddr(obj interface{}) ([]string, error) {
	att := obj.(*netv1a1.NetworkAttachment)
	return []string{generateMACAddr(att.Status.AddressVNI, gonet.ParseIP(att.Status.IPv4)).String()}, nil
}

func generateMACAddr(vni uint32, guestIPv4 gonet.IP) gonet.HardwareAddr {
	guestIPBytes := guestIPv4.To4()
	mac := make([]byte, 6, 6)
	mac[5] = byte(vni)
	mac[4] = byte(vni >> 8)
	mac[3] = guestIPBytes[3]
	mac[2] = guestIPBytes[2]
	mac[1] = guestIPBytes[1]
	mac[0] = (byte(vni>>13) & 0xF8) | ((guestIPBytes[0] & 0x02) << 1) | 2
	return mac
}

func generateIfcName(macAddr gonet.HardwareAddr) string {
	return "kos" + strings.Replace(macAddr.String(), ":", "", -1)
}

// aggregateStopChannels returns a channel which
// is closed when either ch1 or ch2 is closed
func aggregateTwoStopChannels(ch1, ch2 <-chan struct{}) chan struct{} {
	aggregateStopCh := make(chan struct{})
	go func() {
		select {
		case _, ch1Open := <-ch1:
			if !ch1Open {
				close(aggregateStopCh)
				return
			}
		case _, ch2Open := <-ch2:
			if !ch2Open {
				close(aggregateStopCh)
				return
			}
		}
	}()
	return aggregateStopCh
}

func aggregateErrors(sep string, errs ...error) error {
	aggregateErrsSlice := make([]string, 0, len(errs))
	for i, err := range errs {
		if err != nil && strings.Trim(err.Error(), " ") != "" {
			aggregateErrsSlice = append(aggregateErrsSlice, fmt.Sprintf("error nbr. %d ", i)+err.Error())
		}
	}
	if len(aggregateErrsSlice) > 0 {
		return fmt.Errorf("%s", strings.Join(aggregateErrsSlice, sep))
	}
	return nil
}

// TODO consider switching to pointers wrt value for the interface
func ifcNeedsUpdate(ifcHostIP, newHostIP gonet.IP, ifcMAC, newMAC gonet.HardwareAddr) bool {
	return !ifcHostIP.Equal(newHostIP) || !bytes.Equal(ifcMAC, newMAC)
}
