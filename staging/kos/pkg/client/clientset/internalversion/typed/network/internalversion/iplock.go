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

// Code generated by client-gen. DO NOT EDIT.

package internalversion

import (
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
	network "k8s.io/examples/staging/kos/pkg/apis/network"
	scheme "k8s.io/examples/staging/kos/pkg/client/clientset/internalversion/scheme"
)

// IPLocksGetter has a method to return a IPLockInterface.
// A group's client should implement this interface.
type IPLocksGetter interface {
	IPLocks(namespace string) IPLockInterface
}

// IPLockInterface has methods to work with IPLock resources.
type IPLockInterface interface {
	Create(*network.IPLock) (*network.IPLock, error)
	Update(*network.IPLock) (*network.IPLock, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*network.IPLock, error)
	List(opts v1.ListOptions) (*network.IPLockList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *network.IPLock, err error)
	IPLockExpansion
}

// iPLocks implements IPLockInterface
type iPLocks struct {
	client rest.Interface
	ns     string
}

// newIPLocks returns a IPLocks
func newIPLocks(c *NetworkClient, namespace string) *iPLocks {
	return &iPLocks{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the iPLock, and returns the corresponding iPLock object, and an error if there is any.
func (c *iPLocks) Get(name string, options v1.GetOptions) (result *network.IPLock, err error) {
	result = &network.IPLock{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("iplocks").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of IPLocks that match those selectors.
func (c *iPLocks) List(opts v1.ListOptions) (result *network.IPLockList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &network.IPLockList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("iplocks").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested iPLocks.
func (c *iPLocks) Watch(opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("iplocks").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch()
}

// Create takes the representation of a iPLock and creates it.  Returns the server's representation of the iPLock, and an error, if there is any.
func (c *iPLocks) Create(iPLock *network.IPLock) (result *network.IPLock, err error) {
	result = &network.IPLock{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("iplocks").
		Body(iPLock).
		Do().
		Into(result)
	return
}

// Update takes the representation of a iPLock and updates it. Returns the server's representation of the iPLock, and an error, if there is any.
func (c *iPLocks) Update(iPLock *network.IPLock) (result *network.IPLock, err error) {
	result = &network.IPLock{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("iplocks").
		Name(iPLock.Name).
		Body(iPLock).
		Do().
		Into(result)
	return
}

// Delete takes name of the iPLock and deletes it. Returns an error if one occurs.
func (c *iPLocks) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("iplocks").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *iPLocks) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	var timeout time.Duration
	if listOptions.TimeoutSeconds != nil {
		timeout = time.Duration(*listOptions.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("iplocks").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Timeout(timeout).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched iPLock.
func (c *iPLocks) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *network.IPLock, err error) {
	result = &network.IPLock{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("iplocks").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
