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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
	v1alpha1 "k8s.io/examples/staging/kos/pkg/apis/network/v1alpha1"
)

// FakeIPLocks implements IPLockInterface
type FakeIPLocks struct {
	Fake *FakeNetworkV1alpha1
	ns   string
}

var iplocksResource = schema.GroupVersionResource{Group: "network.example.com", Version: "v1alpha1", Resource: "iplocks"}

var iplocksKind = schema.GroupVersionKind{Group: "network.example.com", Version: "v1alpha1", Kind: "IPLock"}

// Get takes name of the iPLock, and returns the corresponding iPLock object, and an error if there is any.
func (c *FakeIPLocks) Get(name string, options v1.GetOptions) (result *v1alpha1.IPLock, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(iplocksResource, c.ns, name), &v1alpha1.IPLock{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.IPLock), err
}

// List takes label and field selectors, and returns the list of IPLocks that match those selectors.
func (c *FakeIPLocks) List(opts v1.ListOptions) (result *v1alpha1.IPLockList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(iplocksResource, iplocksKind, c.ns, opts), &v1alpha1.IPLockList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.IPLockList{}
	for _, item := range obj.(*v1alpha1.IPLockList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested iPLocks.
func (c *FakeIPLocks) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(iplocksResource, c.ns, opts))

}

// Create takes the representation of a iPLock and creates it.  Returns the server's representation of the iPLock, and an error, if there is any.
func (c *FakeIPLocks) Create(iPLock *v1alpha1.IPLock) (result *v1alpha1.IPLock, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(iplocksResource, c.ns, iPLock), &v1alpha1.IPLock{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.IPLock), err
}

// Update takes the representation of a iPLock and updates it. Returns the server's representation of the iPLock, and an error, if there is any.
func (c *FakeIPLocks) Update(iPLock *v1alpha1.IPLock) (result *v1alpha1.IPLock, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(iplocksResource, c.ns, iPLock), &v1alpha1.IPLock{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.IPLock), err
}

// Delete takes name of the iPLock and deletes it. Returns an error if one occurs.
func (c *FakeIPLocks) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(iplocksResource, c.ns, name), &v1alpha1.IPLock{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeIPLocks) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(iplocksResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.IPLockList{})
	return err
}

// Patch applies the patch and returns the patched iPLock.
func (c *FakeIPLocks) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.IPLock, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(iplocksResource, c.ns, name, data, subresources...), &v1alpha1.IPLock{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.IPLock), err
}
