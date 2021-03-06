/**
 * Copyright (C) 2015 Red Hat, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *         http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package prune

import (
	"fmt"
	"sort"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	kapi "k8s.io/kubernetes/pkg/apis/core"

	appsapi "github.com/openshift/origin/pkg/apps/apis/apps"
	appsutil "github.com/openshift/origin/pkg/apps/util"
)

type mockResolver struct {
	items []*kapi.ReplicationController
	err   error
}

func (m *mockResolver) Resolve() ([]*kapi.ReplicationController, error) {
	return m.items, m.err
}

func TestMergeResolver(t *testing.T) {
	resolverA := &mockResolver{
		items: []*kapi.ReplicationController{
			mockDeployment("a", "b", nil),
		},
	}
	resolverB := &mockResolver{
		items: []*kapi.ReplicationController{
			mockDeployment("c", "d", nil),
		},
	}
	resolver := &mergeResolver{resolvers: []Resolver{resolverA, resolverB}}
	results, err := resolver.Resolve()
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Unexpected results %v", results)
	}
	expectedNames := sets.NewString("b", "d")
	for _, item := range results {
		if !expectedNames.Has(item.Name) {
			t.Errorf("Unexpected name %v", item.Name)
		}
	}
}

func TestOrphanDeploymentResolver(t *testing.T) {
	activeDeploymentConfig := mockDeploymentConfig("a", "active-deployment-config")
	inactiveDeploymentConfig := mockDeploymentConfig("a", "inactive-deployment-config")

	deploymentConfigs := []*appsapi.DeploymentConfig{activeDeploymentConfig}
	deployments := []*kapi.ReplicationController{}

	expectedNames := sets.String{}
	deploymentStatusOptions := []appsapi.DeploymentStatus{
		appsapi.DeploymentStatusComplete,
		appsapi.DeploymentStatusFailed,
		appsapi.DeploymentStatusNew,
		appsapi.DeploymentStatusPending,
		appsapi.DeploymentStatusRunning,
	}

	deploymentStatusFilter := []appsapi.DeploymentStatus{
		appsapi.DeploymentStatusComplete,
		appsapi.DeploymentStatusFailed,
	}
	deploymentStatusFilterSet := sets.String{}
	for _, deploymentStatus := range deploymentStatusFilter {
		deploymentStatusFilterSet.Insert(string(deploymentStatus))
	}

	for _, deploymentStatusOption := range deploymentStatusOptions {
		deployments = append(deployments, withStatus(mockDeployment("a", string(deploymentStatusOption)+"-active", activeDeploymentConfig), deploymentStatusOption))
		deployments = append(deployments, withStatus(mockDeployment("a", string(deploymentStatusOption)+"-inactive", inactiveDeploymentConfig), deploymentStatusOption))
		deployments = append(deployments, withStatus(mockDeployment("a", string(deploymentStatusOption)+"-orphan", nil), deploymentStatusOption))
		if deploymentStatusFilterSet.Has(string(deploymentStatusOption)) {
			expectedNames.Insert(string(deploymentStatusOption) + "-inactive")
			expectedNames.Insert(string(deploymentStatusOption) + "-orphan")
		}
	}

	dataSet := NewDataSet(deploymentConfigs, deployments)
	resolver := NewOrphanDeploymentResolver(dataSet, deploymentStatusFilter)
	results, err := resolver.Resolve()
	if err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	foundNames := sets.String{}
	for _, result := range results {
		foundNames.Insert(result.Name)
	}
	if len(foundNames) != len(expectedNames) || !expectedNames.HasAll(foundNames.List()...) {
		t.Errorf("expected %v, actual %v", expectedNames, foundNames)
	}
}

func TestPerDeploymentConfigResolver(t *testing.T) {
	deploymentStatusOptions := []appsapi.DeploymentStatus{
		appsapi.DeploymentStatusComplete,
		appsapi.DeploymentStatusFailed,
		appsapi.DeploymentStatusNew,
		appsapi.DeploymentStatusPending,
		appsapi.DeploymentStatusRunning,
	}
	deploymentConfigs := []*appsapi.DeploymentConfig{
		mockDeploymentConfig("a", "deployment-config-1"),
		mockDeploymentConfig("b", "deployment-config-2"),
	}
	deploymentsPerStatus := 100
	deployments := []*kapi.ReplicationController{}
	for _, deploymentConfig := range deploymentConfigs {
		for _, deploymentStatusOption := range deploymentStatusOptions {
			for i := 0; i < deploymentsPerStatus; i++ {
				deployment := withStatus(mockDeployment(deploymentConfig.Namespace, fmt.Sprintf("%v-%v-%v", deploymentConfig.Name, deploymentStatusOption, i), deploymentConfig), deploymentStatusOption)
				deployments = append(deployments, deployment)
			}
		}
	}

	now := metav1.Now()
	for i := range deployments {
		creationTimestamp := metav1.NewTime(now.Time.Add(-1 * time.Duration(i) * time.Hour))
		deployments[i].CreationTimestamp = creationTimestamp
	}

	// test number to keep at varying ranges
	for keep := 0; keep < deploymentsPerStatus*2; keep++ {
		dataSet := NewDataSet(deploymentConfigs, deployments)

		expectedNames := sets.String{}
		deploymentCompleteStatusFilterSet := sets.NewString(string(appsapi.DeploymentStatusComplete))
		deploymentFailedStatusFilterSet := sets.NewString(string(appsapi.DeploymentStatusFailed))

		for _, deploymentConfig := range deploymentConfigs {
			deploymentItems, err := dataSet.ListDeploymentsByDeploymentConfig(deploymentConfig)
			if err != nil {
				t.Errorf("Unexpected err %v", err)
			}
			completedDeployments, failedDeployments := []*kapi.ReplicationController{}, []*kapi.ReplicationController{}
			for _, deployment := range deploymentItems {
				status := deployment.Annotations[appsapi.DeploymentStatusAnnotation]
				if deploymentCompleteStatusFilterSet.Has(status) {
					completedDeployments = append(completedDeployments, deployment)
				} else if deploymentFailedStatusFilterSet.Has(status) {
					failedDeployments = append(failedDeployments, deployment)
				}
			}
			sort.Sort(appsutil.ByMostRecent(completedDeployments))
			sort.Sort(appsutil.ByMostRecent(failedDeployments))
			purgeCompleted := []*kapi.ReplicationController{}
			purgeFailed := []*kapi.ReplicationController{}
			if keep >= 0 && keep < len(completedDeployments) {
				purgeCompleted = completedDeployments[keep:]
			}
			if keep >= 0 && keep < len(failedDeployments) {
				purgeFailed = failedDeployments[keep:]
			}
			for _, deployment := range purgeCompleted {
				expectedNames.Insert(deployment.Name)
			}
			for _, deployment := range purgeFailed {
				expectedNames.Insert(deployment.Name)
			}
		}

		resolver := NewPerDeploymentConfigResolver(dataSet, keep, keep)
		results, err := resolver.Resolve()
		if err != nil {
			t.Errorf("Unexpected error %v", err)
		}
		foundNames := sets.String{}
		for _, result := range results {
			foundNames.Insert(result.Name)
		}
		if len(foundNames) != len(expectedNames) || !expectedNames.HasAll(foundNames.List()...) {
			expectedValues := expectedNames.List()
			actualValues := foundNames.List()
			sort.Strings(expectedValues)
			sort.Strings(actualValues)
			t.Errorf("keep %v\n, expected \n\t%v\n, actual \n\t%v\n", keep, expectedValues, actualValues)
		}
	}
}
