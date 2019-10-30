/*
Copyright 2019 The Kubernetes Authors.

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

package subnet

import (
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	k8stesting "k8s.io/client-go/testing"
	k8scache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	netv1a1 "k8s.io/examples/staging/kos/pkg/apis/network/v1alpha1"
	kosfake "k8s.io/examples/staging/kos/pkg/client/clientset/versioned/fake"
	koscsv1a1 "k8s.io/examples/staging/kos/pkg/client/clientset/versioned/typed/network/v1alpha1"
	kosinformers "k8s.io/examples/staging/kos/pkg/client/informers/externalversions"
)

const (
	// Values used to initialize test subnets.
	ns1, ns2     = "ns1", "ns2"
	name1, name2 = "s1", "s2"
	cidr1, cidr2 = "192.168.10.0/24", "192.168.11.0/24"
	vni1, vni2   = 1, 2
	uid1, uid2   = "1", "2"
	rv           = "1"
)

// Each subnet validator test goes through a sequence of one or more rounds, and
// each round has its own expected output.
type validatorTestRound struct {
	// The subnets that we expect to exist in the API server at the end of the
	// test round. This is the expected output.
	expectedSubnets []netv1a1.Subnet

	// The function to invoke before transitioning to the next round. This
	// typically deletes/creates a subnet, so that the next round can check that
	// the validator correctly reconsidered the other subnets.
	transitionToNextRound func(koscsv1a1.NetworkV1alpha1Interface) error
}

type validatorTestCase struct {
	// Subnets that exist in the API server when the test starts.
	initialSubnets []runtime.Object

	// Each test has one or more rounds, each round has its own expected output.
	rounds []validatorTestRound

	// reaction can be used to inject failures in the API calls the subnet
	// validator does.
	reaction k8stesting.ReactionFunc
}

func TestSubnetValidator(t *testing.T) {
	testCases := map[string]validatorTestCase{
		// Test cases checking that valid is monotonic.
		"only existing subnet is valid, stays valid": {
			initialSubnets: []runtime.Object{
				newSubnet1(valid),
			},
			rounds: []validatorTestRound{
				{
					expectedSubnets: []netv1a1.Subnet{
						*newSubnet1(valid),
					},
				},
			},
		},

		// Test cases checking that valid subnets become valid.
		"only existing subnet becomes valid": {
			initialSubnets: []runtime.Object{
				newSubnet1(),
			},
			rounds: []validatorTestRound{
				{
					expectedSubnets: []netv1a1.Subnet{
						*newSubnet1(valid),
					},
				},
			},
		},
		"two subnets with different VNIs become valid": {
			initialSubnets: []runtime.Object{
				newSubnet1(),
				newSubnet2(),
			},
			rounds: []validatorTestRound{
				{
					expectedSubnets: []netv1a1.Subnet{
						*newSubnet1(valid),
						*newSubnet2(valid),
					},
				},
			},
		},
		"two non-conflicting subnets with same VNI become valid": {
			initialSubnets: []runtime.Object{
				newSubnet1(),
				newSubnet2(ns(ns1), vni(vni1)),
			},
			rounds: []validatorTestRound{
				{
					expectedSubnets: []netv1a1.Subnet{
						*newSubnet1(valid),
						*newSubnet2(ns(ns1), vni(vni1), valid),
					},
				},
			},
		},

		// Test cases checking that invalid subnets become invalid.
		"two CIDR-conflicting subnets become invalid": {
			initialSubnets: []runtime.Object{
				newSubnet1(),
				newSubnet1(uid(uid2), name(name2)),
			},
			rounds: []validatorTestRound{
				{
					expectedSubnets: []netv1a1.Subnet{
						*newSubnet1(errMsg),
						*newSubnet1(uid(uid2), name(name2), errMsg),
					},
				},
			},
		},

		// Test cases checking that the subnet validator reacts correctly to
		// transient failures in API calls.
		"valid subnet status update fails; it is retried": {
			initialSubnets: []runtime.Object{
				newSubnet1(),
			},
			rounds: []validatorTestRound{
				{
					expectedSubnets: []netv1a1.Subnet{
						*newSubnet1(valid),
					},
				},
			},
			// Make the update fail twice.
			reaction: newUpdateErrorReaction(2),
		},

		// Test cases checking that previously evaluated subnets are re-evaluated
		// if a relevant event takes place.
		"invalid subnet's only rival is deleted, subnet becomes valid": {
			initialSubnets: []runtime.Object{
				newSubnet1(),
				newSubnet1(uid(uid2), name(name2)),
			},
			rounds: []validatorTestRound{
				{
					// round 0
					expectedSubnets: []netv1a1.Subnet{
						*newSubnet1(errMsg),
						*newSubnet1(uid(uid2), name(name2), errMsg),
					},
					transitionToNextRound: func(client koscsv1a1.NetworkV1alpha1Interface) error {
						// Delete one of the two subnets so that the other becomes valid.
						return client.Subnets(ns1).Delete(name2, &k8smetav1.DeleteOptions{})
					},
				},
				{
					// round 1
					expectedSubnets: []netv1a1.Subnet{
						*newSubnet1(valid),
					},
				},
			},
		},
	}

	// By default go-cmp considers equals two slices only if they contain the
	// same items in the same order. This is too strict when comparing
	// `Status.Errors` in expected and got subnets. cmpSubnetStatusErrs is a
	// function that claims equality between `Status.Errors` of two subnets in a
	// more liberal way.
	cmpSubnetStatusErrs := cmp.Comparer(func(errs1, errs2 []string) bool {
		return (len(errs1) > 0 && len(errs2) > 0) || (len(errs1) == 0 && len(errs2) == 0)
	})

	// To compare expected and got subnets we use go-cmp but to have more robust
	// and concise tests we define options to tweak the way go-cmp determines
	// equality.
	diffOptions := cmp.Options{
		// Skip `ObjectMeta.ResourceVersion` when comparing expected and got
		// subnets to avoid specifying the correct value in the expected subnets.
		cmpopts.IgnoreFields(netv1a1.Subnet{}, "ResourceVersion"),

		// go-cmp considers slices equals only if they contain the same elements
		// in the same order, but we have no control on the order of the slice
		// of subnets which represents the test output. We don't want spurius
		// test failures due to the slice of expected subnets being in a
		// different order wrt the slice of got subnets, so we tell go-cmp to
		// sort slices before comparing them.
		cmpopts.SortSlices(func(s1, s2 netv1a1.Subnet) bool {
			// Sort on UIDs, but any field unique over subnets would work.
			return s1.UID < s2.UID
		}),

		// `SubnetStatus.Errors` has two purposes:
		//
		// (1) Conveying information to users on why a subnet is invalid.
		//
		// (2) Telling controllers that read subnets (currently only the IPAM)
		//     whether a subnet with status.validated=false has been processed
		//     by the validator and deemed invalid, or is yet to be processed
		//     by the validator.
		//
		// Currently we test only (2), meaning that tests don't care about
		// what's written in `SubnetStatus.Errors`, they only care whether it's
		// set or not. To achieve this we tweak go-cmp accordingly by passing
		// `cmpSubnetStatusErrs`, our custom Equality judge for
		// `SubnetStatus.Errors`.
		cmp.FilterPath(func(fieldPath cmp.Path) bool {
			return fieldPath.String() == "Status.Errors"
		}, cmpSubnetStatusErrs),
	}

	for description, tc := range testCases {
		t.Run(description, func(t *testing.T) {
			parallelTest(tc, diffOptions, t)
		})
	}
}

func parallelTest(tc validatorTestCase, diffOptions cmp.Options, t *testing.T) {
	// There's a lower bound (=~ 1220 ms) to the execution time of each test,
	// running them sequentially would be slow, so we run them in parallel.
	t.Parallel()

	// Put the test initial subnets in the clientset (AKA the fake API server).
	client := kosfake.NewSimpleClientset(tc.initialSubnets...)

	// Make incRVOnUpdate intercept subnet updates. incRVOnUpdate increases the
	// resource version of the updated subnet before the subnet is "persisted".
	// This is needed because the fake client used in tests ignores resource
	// versions, but the subnet validator uses them to assess (in)equality
	// between different versions of the same subnet (i.e. most tests would
	// incorrectly fail if `ObjectMeta.ResourceVersion` didn't change after an
	// update).
	client.PrependReactor("update", "subnets", incRVOnUpdate)

	// If the test case has a custom reaction to updates, prepend that as well.
	if tc.reaction != nil {
		client.PrependReactor("update", "subnets", tc.reaction)
	}

	subnetsInformer := kosinformers.NewSharedInformerFactory(client, 0).Network().V1alpha1().Subnets()
	subnetValidator := NewValidationController(client.NetworkV1alpha1(),
		subnetsInformer.Informer(),
		subnetsInformer.Lister(),
		nil,
		// Use a fake rate limiter (delay is always 0) to reduce likelyhood of
		// spurius failures caused by a test timeout.
		workqueue.NewRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(0, 0)),
		// IMO we don't want more than one worker, it would make test failures
		// less reproducible.
		1,
		"",
		true)

	stopCh := make(chan struct{})
	// Stop the informer and the subnet validator when the test ends.
	defer close(stopCh)
	go subnetsInformer.Informer().Run(stopCh)
	go subnetValidator.Run(stopCh)

	// backoff determines the instants at which we sample the subnet population
	// and compare it to the expected subnet population. Because the test passes
	// after three consecutive matches between the sampled subnets and the
	// expected ones, this backoff determines a lower bound on a single test
	// execution time of roughly
	// 500 ms + (500 ms * 0.8) + ((500 ms * 0.8) * 0.8) = 1.22 s.
	// After ten samplings with no three consecutive matches the test fails (we
	// need a cut-off, otherwise failing tests can hang indefinitely).
	//
	// TODO think whether these are sensible values. If not, think about
	// sensible values (the current behavior is "start slow, end fast",
	// we might want the exact opposite if we believe that the validator
	// produces the expected output quickly more often than not).
	backoff := wait.Backoff{
		Duration: 500 * time.Millisecond,
		Factor:   0.8,
		Steps:    10,
	}

	// Start sampling the subnet population to compare it to the expected
	// subnets only after the informer's caches have synced as some tests
	// can only produce the desired output after processing each subnet
	// at least once.
	if !k8scache.WaitForCacheSync(stopCh, subnetsInformer.Informer().HasSynced) {
		t.Fatalf("subnets informer cache failed to sync")
	}

	subnetsIfc := client.NetworkV1alpha1().Subnets(k8smetav1.NamespaceAll)

	for i, round := range tc.rounds {
		diff := ""
		consecutiveMatches := 0
		err := wait.ExponentialBackoff(backoff, func() (bool, error) {
			// Sample the subnet population.
			observedSubnets, err := subnetsIfc.List(k8smetav1.ListOptions{})
			if err != nil {
				return false, fmt.Errorf("error while retrieving observed subnets: %s", err.Error())
			}

			// Compare expected and got subnets.
			if diff = cmp.Diff(round.expectedSubnets, observedSubnets.Items, diffOptions); diff == "" {
				consecutiveMatches++
				if consecutiveMatches == 3 {
					// Test passes only if expected subnets matched got ones for
					// three consecutive times. This way we gain some confidence
					// that the expected subnets are the steady state.
					return true, nil
				}
			} else {
				consecutiveMatches = 0
			}
			return false, nil
		})
		if err == wait.ErrWaitTimeout {
			t.Fatalf("round %d: difference between expected and observed subnets (+- wrt expected):\n%s\nThis failure might occur even if the validator is correct but too slow to produce the desired output, try running the test multiple times or changing the backoff policy to rule that out.", i, diff)
		} else if err != nil {
			t.Fatalf("round %d: error while comparing expected and observed subnets: %s", i, err)
		}
		if round.transitionToNextRound == nil {
			continue
		}
		if err := round.transitionToNextRound(client.NetworkV1alpha1()); err != nil {
			t.Fatalf("round %d: error while transitioning to next round: %s", i, err)
		}
	}
}

// Test that a subnet with a rival is not marked as valid even if the rival is
// not in the informer. This ensures that two validators running at the same
// time don't mark as valid two conflicting subnets (with each validator marking
// as valid one subnet) if the two subnets appear in the validators' informers
// in opposite orders.
// TODO This unit test is in dissonance with the story the rest of this file tells:
// It does not use the testing scaffolding used by other tests and it exposes more
// implementation wrt the other tests. Think whether we can harmonize.
func TestSubnetValidator_lateInformer(t *testing.T) {
	// s1 is the subnet that will be processed.
	s1 := newSubnet1()

	// s2 is s1's rival, their CIDRs overlap.
	s2 := newSubnet1(uid(uid2), name(name2))

	// Put s1 and s2 in the client (AKA the fake API server).
	client := kosfake.NewSimpleClientset(s1, s2)

	subnetsInformer := kosinformers.NewSharedInformerFactory(client, 0).Network().V1alpha1().Subnets()

	// Put only s1 in the informer: this allows simulating the scenario where s1
	// is processed with s2 not yet in the informer.
	subnetsInformer.Informer().GetStore().Add(s1)

	subnetValidator := NewValidationController(client.NetworkV1alpha1(),
		subnetsInformer.Informer(),
		subnetsInformer.Lister(),
		nil,
		workqueue.NewRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(0, 0)),
		0,
		"",
		true)

	// Process s1. Ignore the error that will be returned (assuming the
	// validator is correct) since in the current implementation successful
	// processing of a subnet is conditional to the presence of data structures
	// initialized when rival subnets are processed (and s2 is never processed).
	// TODO: Ignoring the error gives away a lot of the implementation.
	subnetValidator.processSubnet(k8stypes.NamespacedName{s1.Namespace, s1.Name})

	// Retrieve s1 after processing it.
	gotS1, err := client.NetworkV1alpha1().Subnets(s1.Namespace).Get(s1.Name, k8smetav1.GetOptions{})
	if err != nil {
		t.Fatalf("error while retrieving s1: %s", err.Error())
	}

	// s1 MUST NOT be validated even if its only rival is not in the informer.
	if gotS1.Status.Validated {
		t.Fatalf("subnet was marked as valid even if rival exists")
	}
}

// incRVOnUpdate assumes that `action` is a subnet update and that the subnet's
// resource version can be parsed into an int, never call it if such conditions
// are not met.
func incRVOnUpdate(action k8stesting.Action) (bool, runtime.Object, error) {
	update := action.(k8stesting.UpdateAction)
	subnet := update.GetObject().(*netv1a1.Subnet)
	rv, _ := strconv.Atoi(subnet.ResourceVersion)
	rv++
	subnet.ResourceVersion = strconv.Itoa(rv)
	return false, nil, nil
}

// newUpdateErrorReaction can be registered to intercept updates from the fake
// client used for tests and return a failure. It allows testing how the
// validator reacts to updates failures.
func newUpdateErrorReaction(numberOfErrors int) k8stesting.ReactionFunc {
	i := 0
	return func(action k8stesting.Action) (bool, runtime.Object, error) {
		if i < numberOfErrors {
			i++
			return true, nil, errors.New("test error")
		}
		return false, nil, nil
	}
}

//****************************************************************************
// Follow functional options and util functions to create test subnets with  //
// conveniently initialized values.                                          //
//****************************************************************************

type option func(*netv1a1.Subnet)

func newSubnet1(opts ...option) *netv1a1.Subnet {
	return newSubnet(ns1, name1, cidr1, vni1, uid1, opts...)
}

func newSubnet2(opts ...option) *netv1a1.Subnet {
	return newSubnet(ns2, name2, cidr2, vni2, uid2, opts...)
}

func newSubnet(ns, name, cidr string, vni uint32, uid k8stypes.UID, opts ...option) *netv1a1.Subnet {
	s := &netv1a1.Subnet{}
	s.Namespace = ns
	s.Name = name
	s.Spec.IPv4 = cidr
	s.Spec.VNI = vni
	s.UID = uid
	s.ResourceVersion = rv

	for _, applyOption := range opts {
		applyOption(s)
	}

	return s
}

func valid(s *netv1a1.Subnet) {
	s.Status.Validated = true
}

func vni(vni uint32) option {
	return func(s *netv1a1.Subnet) {
		s.Spec.VNI = vni
	}
}

func cidr(cidr string) option {
	return func(s *netv1a1.Subnet) {
		s.Spec.IPv4 = cidr
	}
}

func ns(ns string) option {
	return func(s *netv1a1.Subnet) {
		s.Namespace = ns
	}
}

func name(name string) option {
	return func(s *netv1a1.Subnet) {
		s.Name = name
	}
}

func uid(uid k8stypes.UID) option {
	return func(s *netv1a1.Subnet) {
		s.UID = uid
	}
}

func errMsg(s *netv1a1.Subnet) {
	s.Status.Errors = []string{"error message"}
}
