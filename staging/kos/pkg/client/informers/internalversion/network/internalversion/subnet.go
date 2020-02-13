/*
Copyright 2020 The Kubernetes Authors.

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

// Code generated by informer-gen. DO NOT EDIT.

package internalversion

import (
	time "time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
	network "k8s.io/examples/staging/kos/pkg/apis/network"
	clientsetinternalversion "k8s.io/examples/staging/kos/pkg/client/clientset/internalversion"
	internalinterfaces "k8s.io/examples/staging/kos/pkg/client/informers/internalversion/internalinterfaces"
	internalversion "k8s.io/examples/staging/kos/pkg/client/listers/network/internalversion"
)

// SubnetInformer provides access to a shared informer and lister for
// Subnets.
type SubnetInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() internalversion.SubnetLister
}

type subnetInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewSubnetInformer constructs a new informer for Subnet type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewSubnetInformer(client clientsetinternalversion.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredSubnetInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredSubnetInformer constructs a new informer for Subnet type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredSubnetInformer(client clientsetinternalversion.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.Network().Subnets(namespace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.Network().Subnets(namespace).Watch(options)
			},
		},
		&network.Subnet{},
		resyncPeriod,
		indexers,
	)
}

func (f *subnetInformer) defaultInformer(client clientsetinternalversion.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredSubnetInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *subnetInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&network.Subnet{}, f.defaultInformer)
}

func (f *subnetInformer) Lister() internalversion.SubnetLister {
	return internalversion.NewSubnetLister(f.Informer().GetIndexer())
}
