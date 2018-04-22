/*
Copyright 2017 The Kubernetes Authors.

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

package pluginregistration

import (
	"time"
	"net"
	"fmt"
	"strings"

	"k8s.io/api/core/v1"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
)

type Validator interface {
	Connect(socketPath string, timeout time.Duration) (*grpc.ClientConn, error)
	ValidateEndpoint(*grpc.ClientConn) (string, error)

	// Will try to notify the endpoint
	NotifyRegistrationStatus(*grpc.ClientConn, error)
}

type ValidatorImpl struct {
	version string
	domain string
}

func NewValidatorImpl(version string, domain string) *ValidatorImpl{
	return &ValidatorImpl{
		version: version,
		domain: domain,
	}
}

func (v *ValidatorImpl) Connect(socketPath string, timeout time.Duration)  (*grpc.ClientConn, error) {
	c, err := dial(socketPath, timeout)
	return c, err
}

func (v *ValidatorImpl) ValidateEndpoint(c *grpc.ClientConn) (string, error) {
	client := NewIdentityClient(c)

	if err := v.validateVersions(client); err != nil {
		return "", err
	}

	name, err := v.validateIdentity(client)
	if err != nil {
		return "", err
	}

	return name, nil
}

func (v * ValidatorImpl) NotifyRegistrationStatus(c *grpc.ClientConn, err error) {
	client := NewIdentityClient(c)

	ctx, statusCancel := context.WithTimeout(context.Background(), time.Second)
	defer statusCancel()

	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	client.PluginRegistrationStatus(ctx, &RegistrationStatus{
		Success: err == nil,
		Error: errorMsg,
	})
}

func (v * ValidatorImpl) validateVersions(client IdentityClient) error {
	ctx, versionsCancel := context.WithTimeout(context.Background(), time.Second)
	defer versionsCancel()

	versions, err := client.GetSupportedVersions(ctx, &GetSupportedVersionsRequest{})
	if err != nil {
		return err
	}

	for _, supportedVersion := range versions.SupportedVersions {
		if supportedVersion == v.version {
			return nil
		}
	}

	return fmt.Errorf("Plugin does not support current Device Plugin API version: `%s`", v.version)
}

func (v * ValidatorImpl) validateIdentity(client IdentityClient) (string, error) {
	ctx, identityCancel := context.WithTimeout(context.Background(), time.Second)
	defer identityCancel()

	identity, err := client.GetPluginIdentity(ctx, &GetPluginIdentityRequest{Version: v.version})
	if err != nil {
		return "", err
	}

	if !v1helper.IsExtendedResourceName(v1.ResourceName(identity.ResourceName)) {
		return "", fmt.Errorf("Invalid resource name `%s`, it is not an Extended Resource name", identity.ResourceName)
	}

	if !strings.HasPrefix(identity.ResourceName, v.domain) {
		return "", fmt.Errorf("Invalid resource name, expecting to Resource domain name `%s` to be `%s`", v.domain, identity.ResourceName)
	}

	return identity.ResourceName, nil
}

// dial establishes the gRPC communication with the registered plugin.
func dial(unixSocket string, timeout time.Duration) (*grpc.ClientConn, error) {
	c, err := grpc.Dial(unixSocket, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(timeout),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("Failed to dial Device Plugin at %s: %v", unixSocket, err)
	}

	return c, nil
}
