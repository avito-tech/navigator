// +build unit

package k8s

import (
	"context"
	"io/ioutil"
	"testing"
	"time"

	fakecdr "github.com/avito-tech/navigator/pkg/apis/generated/clientset/versioned/fake"
	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	fakekube "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestWatcherStart(t *testing.T) {

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	cs := fakekube.NewSimpleClientset()
	extCs := fakecdr.NewSimpleClientset()
	fakeCache := &MockCache{}
	fakeCanary := &MockCanary{}
	fakeNotifier := &MockNotifier{}
	fakeNexusCache := &MockNexusCache{}
	clusterID := "alpha"

	watcher := NewWatcher(logger, cs, extCs, fakeCache, fakeCanary, fakeNotifier, fakeNexusCache, clusterID)
	ctx, cancel := context.WithCancel(context.Background())

	watcher.Start(ctx)
	type action struct {
		verb     string
		resource string
		got      bool
	}

	test := struct {
		name    string
		actions []action
	}{
		name: "check start working",
		actions: []action{
			{
				verb:     "list",
				resource: "services",
			},
			{
				verb:     "list",
				resource: "endpoints",
			},
			{
				verb:     "list",
				resource: "nexuses",
			},
			{
				verb:     "list",
				resource: "canaryreleases",
			},
			{
				verb:     "watch",
				resource: "services",
			},
			{
				verb:     "watch",
				resource: "endpoints",
			},
			{
				verb:     "watch",
				resource: "nexuses",
			},
			{
				verb:     "watch",
				resource: "canaryreleases",
			},
		},
	}

	t.Run(test.name, func(t *testing.T) {
		ok := true
		for i, act := range test.actions {
			for _, a := range cs.Actions() {
				test.actions[i].got = a.Matches(act.verb, act.resource) || test.actions[i].got
			}
			for _, a := range extCs.Actions() {
				test.actions[i].got = a.Matches(act.verb, act.resource) || test.actions[i].got
			}
			ok = test.actions[i].got && ok
		}

		if !ok {
			t.Errorf("actions: %#v", test.actions)
		}
	})
	cancel()
}

func TestWatcherStartWorking(t *testing.T) {

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	cs := fakekube.NewSimpleClientset()
	extCs := fakecdr.NewSimpleClientset()
	fakeCache := &MockCache{}
	fakeCanary := &MockCanary{}
	fakeNotifier := &MockNotifier{}
	fakeNexusCache := &MockNexusCache{}

	ip := "1.1.1.1"
	clusterID := "alpha"
	weight := 100
	epName := NewQualifiedName("test", "service-test")
	svcName := NewQualifiedName("service", "service")

	callMapping := fakeCanary.On("GetMappings", epName, clusterID)
	callMapping.Return(map[QualifiedName]EndpointMapping{
		svcName: NewEndpointMapping(epName, svcName, 100),
	})

	callBackends := fakeCache.On("UpdateBackends", svcName, clusterID, epName, weight, []string{ip})
	callBackends.Return([]QualifiedName{svcName})

	callNotify := fakeNotifier.On("NotifyServicesUpdated", []QualifiedName{svcName})
	callNotify.Return()

	watcher := NewWatcher(logger, cs, extCs, fakeCache, fakeCanary, fakeNotifier, fakeNexusCache, clusterID)

	ctx, cancel := context.WithCancel(context.Background())

	w := watch.NewFake()
	cs.PrependWatchReactor("endpoints", k8stesting.DefaultWatchReactor(w, nil))

	watcher.Start(ctx)

	tests := []struct {
		name string
		obj  runtime.Object
	}{
		{
			name: "add ep",
			obj: &core.Endpoints{
				ObjectMeta: meta.ObjectMeta{
					Namespace: epName.Namespace,
					Name:      epName.Name,
				},
				Subsets: []core.EndpointSubset{
					{
						Addresses: []core.EndpointAddress{
							{IP: ip},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w.Add(tt.obj)
		})
	}

	time.Sleep(time.Second * 1)
	fakeCache.AssertExpectations(t)
	fakeCanary.AssertExpectations(t)
	fakeNotifier.AssertExpectations(t)
	cancel()
}
