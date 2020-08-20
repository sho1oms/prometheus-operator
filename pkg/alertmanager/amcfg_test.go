// Copyright 2020 The prometheus-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package alertmanager

import (
	"context"
	"fmt"
	"testing"

	"github.com/kylelemons/godebug/pretty"

	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"github.com/prometheus/alertmanager/config"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGenerateConfig(t *testing.T) {
	type testCase struct {
		name       string
		kclient    kubernetes.Interface
		baseConfig alertmanagerConfig
		amConfigs  map[string]*monitoringv1alpha1.AlertmanagerConfig
		expected   string
	}

	testCases := []testCase{
		{
			name:    "skeleton base, no CRs",
			kclient: fake.NewSimpleClientset(),
			baseConfig: alertmanagerConfig{
				Route:     &route{Receiver: "null"},
				Receivers: []*receiver{{Name: "null"}},
			},
			amConfigs: map[string]*monitoringv1alpha1.AlertmanagerConfig{},
			expected: `route:
  receiver: "null"
receivers:
- name: "null"
templates: []
`,
		},
		{
			name:    "base with sub route, no CRs",
			kclient: fake.NewSimpleClientset(),
			baseConfig: alertmanagerConfig{
				Route: &route{
					Receiver: "null",
					Routes: []*route{{
						Receiver: "custom",
					}},
				},
				Receivers: []*receiver{
					{Name: "null"},
					{Name: "custom"},
				},
			},
			amConfigs: map[string]*monitoringv1alpha1.AlertmanagerConfig{},
			expected: `route:
  receiver: "null"
  routes:
  - receiver: custom
receivers:
- name: "null"
- name: custom
templates: []
`,
		},
		{
			name:    "skeleton base, empty CR",
			kclient: fake.NewSimpleClientset(),
			baseConfig: alertmanagerConfig{
				Route:     &route{Receiver: "null"},
				Receivers: []*receiver{{Name: "null"}},
			},
			amConfigs: map[string]*monitoringv1alpha1.AlertmanagerConfig{
				"mynamespace": {},
			},
			expected: `route:
  receiver: "null"
receivers:
- name: "null"
templates: []
`,
		},
		{
			name:    "skeleton base, simple CR",
			kclient: fake.NewSimpleClientset(),
			baseConfig: alertmanagerConfig{
				Route:     &route{Receiver: "null"},
				Receivers: []*receiver{{Name: "null"}},
			},
			amConfigs: map[string]*monitoringv1alpha1.AlertmanagerConfig{
				"mynamespace": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myamc",
						Namespace: "mynamespace",
					},
					Spec: monitoringv1alpha1.AlertmanagerConfigSpec{
						Route: &monitoringv1alpha1.Route{
							Receiver: "test",
						},
						Receivers: []monitoringv1alpha1.Receiver{{Name: "test"}},
					},
				},
			},
			expected: `route:
  receiver: "null"
  routes:
  - receiver: mynamespace-myamc-test
    match:
      namespace: mynamespace
    continue: true
receivers:
- name: "null"
- name: mynamespace-myamc-test
templates: []
`,
		},
		{
			name:    "base with subroute, simple CR",
			kclient: fake.NewSimpleClientset(),
			baseConfig: alertmanagerConfig{
				Route: &route{
					Receiver: "null",
					Routes:   []*route{{Receiver: "null"}},
				},
				Receivers: []*receiver{{Name: "null"}},
			},
			amConfigs: map[string]*monitoringv1alpha1.AlertmanagerConfig{
				"mynamespace": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myamc",
						Namespace: "mynamespace",
					},
					Spec: monitoringv1alpha1.AlertmanagerConfigSpec{
						Route: &monitoringv1alpha1.Route{
							Receiver: "test",
						},
						Receivers: []monitoringv1alpha1.Receiver{{Name: "test"}},
					},
				},
			},
			expected: `route:
  receiver: "null"
  routes:
  - receiver: mynamespace-myamc-test
    match:
      namespace: mynamespace
    continue: true
  - receiver: "null"
receivers:
- name: "null"
- name: mynamespace-myamc-test
templates: []
`,
		},
		{
			name: "CR with Pagerduty Receiver",
			kclient: fake.NewSimpleClientset(
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "am-pd-test-receiver",
						Namespace: "mynamespace",
					},
					Data: map[string][]byte{
						"routingKey": []byte("1234abc"),
					},
				},
			),
			baseConfig: alertmanagerConfig{
				Route: &route{
					Receiver: "null",
				},
				Receivers: []*receiver{{Name: "null"}},
			},
			amConfigs: map[string]*monitoringv1alpha1.AlertmanagerConfig{
				"mynamespace": {
					ObjectMeta: metav1.ObjectMeta{
						Name:      "myamc",
						Namespace: "mynamespace",
					},
					Spec: monitoringv1alpha1.AlertmanagerConfigSpec{
						Route: &monitoringv1alpha1.Route{
							Receiver: "test",
						},
						Receivers: []monitoringv1alpha1.Receiver{{
							Name: "test",
							PagerDutyConfigs: []monitoringv1alpha1.PagerDutyConfig{{
								RoutingKey: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "am-pd-test-receiver",
									},
									Key: "routingKey",
								},
							}},
						}},
					},
				},
			},
			expected: `route:
  receiver: "null"
  routes:
  - receiver: mynamespace-myamc-test
    match:
      namespace: mynamespace
    continue: true
receivers:
- name: "null"
- name: mynamespace-myamc-test
  pagerduty_configs:
  - send_resolved: false
    routing_key: 1234abc
templates: []
`,
		},
	}

	for _, tc := range testCases {
		cg := newConfigGenerator(nil, tc.kclient, nil)
		cfgBytes, err := cg.generateConfig(context.TODO(), tc.baseConfig, tc.amConfigs)
		if err != nil {
			t.Fatal(err)
		}

		result := string(cfgBytes)

		// Verify the generated yaml is as expected
		if result != tc.expected {
			fmt.Println(pretty.Compare(result, tc.expected))
			t.Fatal("generated Alertmanager config does not match expected")
		}

		// Verify the generated config is something that Alertmanager will be happy with
		_, err = config.Load(result)
		if err != nil {
			t.Fatal(err)
		}
	}
}
