/*
Copyright 2021, Pelion IoT and affiliates.
Copyright 2017-2021, CloudNative Labs

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

package utils

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"reflect"
	"testing"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_GetPodCidrFromCniSpec(t *testing.T) {
	testcases := []struct {
		name        string
		cniConfFile string
		podCidr     net.IPNet
		err         error
		filename    string
	}{
		{
			"CNI config file has subnet",
			`{"bridge":"kube-bridge","ipam":{"subnet":"172.17.0.0/24","type":"host-local"},"isDefaultGateway":true,"name":"kubernetes","type":"bridge"}`,
			net.IPNet{
				IP:   net.IP{172, 17, 0, 0},
				Mask: net.CIDRMask(24, 32),
			},
			nil,
			"10-kuberouter.conf",
		},
		{
			"CNI config file missing subnet",
			`{"bridge":"kube-bridge","ipam":{"type":"host-local"},"isDefaultGateway":true,"name":"kubernetes","type":"bridge"}`,
			net.IPNet{},
			nil,
			"10-kuberouter.conf",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			file, err := createFile(testcase.cniConfFile, testcase.filename)
			if err != nil {
				t.Fatalf("Failed to create temporary CNI config file: %v", err)
			}
			defer os.Remove(file.Name())

			cidr, err := GetPodCidrFromCniSpec(file.Name())
			if !reflect.DeepEqual(err, testcase.err) {
				t.Logf("actual error: %v", err)
				t.Logf("expected error: %v", testcase.err)
				t.Error("did not get expected error")
			}

			if !reflect.DeepEqual(cidr, testcase.podCidr) {
				t.Logf("actual pod cidr: %v", cidr)
				t.Logf("expected pod cidr: %v", testcase.podCidr)
				t.Error("did not get expected pod cidr")
			}
		})
	}
}

func Test_InsertPodCidrInCniSpec(t *testing.T) {
	testcases := []struct {
		name        string
		podCidr     string
		existingCni string
		newCni      string
		err         error
		filename    string
	}{
		{
			"insert cidr to cni config",
			"172.17.0.0/24",
			`{"bridge":"kube-bridge","ipam":{"type":"host-local"},"isDefaultGateway":true,"name":"kubernetes","type":"bridge"}`,
			`{"bridge":"kube-bridge","ipam":{"subnet":"172.17.0.0/24","type":"host-local"},"isDefaultGateway":true,"name":"kubernetes","type":"bridge"}`,
			nil,
			"/tmp/10-kuberouter.conf",
		},
		{
			"insert cidr to cni config",
			"172.17.0.0/24",
			`{"cniVersion":"0.3.0","name":"mynet","plugins":[{"bridge":"kube-bridge","ipam":{"type":"host-local"},"isDefaultGateway":true,"name":"kubernetes","type":"bridge"},{"type":"portmap"}]}`,
			`{"cniVersion":"0.3.0","name":"mynet","plugins":[{"bridge":"kube-bridge","ipam":{"subnet":"172.17.0.0/24","type":"host-local"},"isDefaultGateway":true,"name":"kubernetes","type":"bridge"},{"type":"portmap"}]}`,
			nil,
			"/tmp/10-kuberouter.conflist",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			cniConfigFile, err := createFile(testcase.existingCni, testcase.filename)

			if err != nil {
				t.Fatalf("failed to create temporary CNI config: %v", err)
			}
			defer os.Remove(cniConfigFile.Name())

			err = InsertPodCidrInCniSpec(cniConfigFile.Name(), testcase.podCidr)
			if !reflect.DeepEqual(err, testcase.err) {
				t.Logf("actual error: %v", err)
				t.Logf("expected error: %v", err)
				t.Error("did not get expected error")
			}

			newContent, err := readFile(cniConfigFile.Name())
			if err != nil {
				t.Fatalf("failed to read CNI config file: %v", err)
			}

			if newContent != testcase.newCni {
				t.Logf("actual CNI config: %v", newContent)
				t.Logf("expected CNI config: %v", testcase.newCni)
				t.Error("did not get expected CNI config content")
			}
		})
	}
}

func Test_GetPodCidrFromNodeSpec(t *testing.T) {
	testcases := []struct {
		name             string
		hostnameOverride string
		existingNode     *apiv1.Node
		podCIDR          string
		err              error
	}{
		{
			"node with node.Spec.PodCIDR",
			"test-node",
			&apiv1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Spec: apiv1.NodeSpec{
					PodCIDR: "172.17.0.0/24",
				},
			},
			"172.17.0.0/24",
			nil,
		},
		{
			"node with node.Annotations['kube-router.io/pod-cidr']",
			"test-node",
			&apiv1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Annotations: map[string]string{
						podCIDRAnnotation: "172.17.0.0/24",
					},
				},
			},
			"172.17.0.0/24",
			nil,
		},
		{
			"node with invalid pod cidr in node.Annotations['kube-router.io/pod-cidr']",
			"test-node",
			&apiv1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
					Annotations: map[string]string{
						podCIDRAnnotation: "172.17.0.0",
					},
				},
			},
			"",
			errors.New("error parsing pod CIDR in node annotation: invalid CIDR address: 172.17.0.0"),
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			_, err := clientset.CoreV1().Nodes().Create(context.Background(), testcase.existingNode, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("failed to create existing nodes for test: %v", err)
			}

			podCIDR, err := GetPodCidrFromNodeSpec(clientset, testcase.hostnameOverride, "")
			if !reflect.DeepEqual(err, testcase.err) {
				t.Logf("actual error: %v", err)
				t.Logf("expected error: %v", testcase.err)
				t.Error("did not get expected error")
			}

			if podCIDR != testcase.podCIDR {
				t.Logf("actual podCIDR: %q", podCIDR)
				t.Logf("expected podCIDR: %q", testcase.podCIDR)
				t.Error("did not get expected podCIDR")
			}
		})
	}
}

func Test_GetPodCIDRFromArgument(t *testing.T) {
	type testCase struct {
		name             string
		hostnameOverride string
		existingNode     *apiv1.Node
		podCIDR          string
		err              error
	}

	test := &testCase{
		"node with podCIDR pre-defined",
		"test-node",
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
			},
			Spec: apiv1.NodeSpec{
				PodCIDR: "172.18.0.0/24",
			},
		},
		"172.17.0.0/24",
		nil,
	}

	t.Run(test.name, func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		_, err := clientset.CoreV1().Nodes().Create(context.Background(), test.existingNode, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("failed to create existing nodes for test: %v", err)
		}

		podCIDR, err := GetPodCidrFromNodeSpec(clientset, test.hostnameOverride, test.podCIDR)
		if !reflect.DeepEqual(err, test.err) {
			t.Logf("actual error: %v", err)
			t.Logf("expected error: %v", test.err)
			t.Error("did not get expected error")
		}

		if podCIDR != test.podCIDR {
			t.Logf("actual podCIDR: %q", podCIDR)
			t.Logf("expected podCIDR: %q", test.podCIDR)
			t.Error("did not get expected podCIDR")
		}

		if podCIDR == test.existingNode.Spec.PodCIDR {
			t.Logf("node.spec podCIDR: %q", test.existingNode.Spec.PodCIDR)
			t.Logf("expected podCIDR from argument: %q", test.podCIDR)
			t.Error("did not get expected podCIDR")
		}
	})
}

func createFile(content, filename string) (*os.File, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("cannot create file: %v", err)
	}

	if _, err = file.Write([]byte(content)); err != nil {
		return nil, fmt.Errorf("cannot write to file: %v", err)
	}

	fmt.Println("File is ", file.Name())
	return file, nil
}

func readFile(filename string) (string, error) {
	content, err := ioutil.ReadFile(filename)
	return string(content), err
}
