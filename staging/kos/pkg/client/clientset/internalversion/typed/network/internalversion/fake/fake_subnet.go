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
	network "k8s.io/examples/staging/kos/pkg/apis/network"
)

// FakeSubnets implements SubnetInterface
type FakeSubnets struct {
	Fake *FakeNetwork
	ns   string
}

var subnetsResource = schema.GroupVersionResource{Group: "network.example.com", Version: "", Resource: "subnets"}

var subnetsKind = schema.GroupVersionKind{Group: "network.example.com", Version: "", Kind: "Subnet"}

// Get takes name of the subnet, and returns the corresponding subnet object, and an error if there is any.
func (c *FakeSubnets) Get(name string, options v1.GetOptions) (result *network.Subnet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(subnetsResource, c.ns, name), &network.Subnet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*network.Subnet), err
}

// List takes label and field selectors, and returns the list of Subnets that match those selectors.
func (c *FakeSubnets) List(opts v1.ListOptions) (result *network.SubnetList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(subnetsResource, subnetsKind, c.ns, opts), &network.SubnetList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &network.SubnetList{}
	for _, item := range obj.(*network.SubnetList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested subnets.
func (c *FakeSubnets) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(subnetsResource, c.ns, opts))

}

// Create takes the representation of a subnet and creates it.  Returns the server's representation of the subnet, and an error, if there is any.
func (c *FakeSubnets) Create(subnet *network.Subnet) (result *network.Subnet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(subnetsResource, c.ns, subnet), &network.Subnet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*network.Subnet), err
}

// Update takes the representation of a subnet and updates it. Returns the server's representation of the subnet, and an error, if there is any.
func (c *FakeSubnets) Update(subnet *network.Subnet) (result *network.Subnet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(subnetsResource, c.ns, subnet), &network.Subnet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*network.Subnet), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeSubnets) UpdateStatus(subnet *network.Subnet) (*network.Subnet, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(subnetsResource, "status", c.ns, subnet), &network.Subnet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*network.Subnet), err
}

// Delete takes name of the subnet and deletes it. Returns an error if one occurs.
func (c *FakeSubnets) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(subnetsResource, c.ns, name), &network.Subnet{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeSubnets) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(subnetsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &network.SubnetList{})
	return err
}

// Patch applies the patch and returns the patched subnet.
func (c *FakeSubnets) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *network.Subnet, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(subnetsResource, c.ns, name, data, subresources...), &network.Subnet{})

	if obj == nil {
		return nil, err
	}
	return obj.(*network.Subnet), err
}
