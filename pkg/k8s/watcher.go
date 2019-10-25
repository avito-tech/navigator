package k8s

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	cache2 "k8s.io/client-go/tools/cache"

	"github.com/avito-tech/navigator/pkg/apis/generated/clientset/versioned"
	navigatorInformer "github.com/avito-tech/navigator/pkg/apis/generated/informers/externalversions/navigator/v1"
)

const ResyncTimeout time.Duration = 0

type notifier interface {
	serviceNotifier
	nexusNotifier
}

type Watcher interface {
	Start(ctx context.Context)
}

type watcher struct {
	logger           logrus.FieldLogger
	informersFactory informers.SharedInformerFactory
	crIndexInformer  cache2.SharedIndexInformer
	depIndexInformer cache2.SharedIndexInformer
}

func NewWatcher(
	logger logrus.FieldLogger,
	cs kubernetes.Interface,
	extCs versioned.Interface,
	k8sCache Cache,
	canary Canary,
	notifier notifier,
	nexusCache NexusCache,
	clusterID string) Watcher {

	informersFactory := informers.NewSharedInformerFactory(cs, ResyncTimeout)
	canaryReleaseInf := navigatorInformer.NewCanaryReleaseInformer(extCs, "", ResyncTimeout, nil)
	nexusInf := navigatorInformer.NewNexusInformer(extCs, "", ResyncTimeout, nil)

	serviceEventHandler := NewServiceEventHandler(
		logger,
		k8sCache,
		notifier,
		clusterID,
	)

	endpointEventHandler := NewEndpointEventHandler(
		logger,
		k8sCache,
		notifier,
		canary,
		clusterID,
	)

	epLister := informersFactory.Core().V1().Endpoints().Lister()
	canaryReleaseEventHandler := NewCanaryReleaseEventHandler(logger, k8sCache, canary, clusterID, epLister, endpointEventHandler)
	nexusHandler := NewNexusEventHandler(logger, nexusCache, notifier, clusterID)

	informersFactory.Core().V1().Services().Informer().AddEventHandler(serviceEventHandler)
	informersFactory.Core().V1().Endpoints().Informer().AddEventHandler(endpointEventHandler)
	canaryReleaseInf.AddEventHandler(canaryReleaseEventHandler)
	nexusInf.AddEventHandler(nexusHandler)

	return &watcher{
		logger:           logger.WithField("context", "k8s.Watcher"),
		informersFactory: informersFactory,
		crIndexInformer:  canaryReleaseInf,
		depIndexInformer: nexusInf,
	}
}

func (w *watcher) Start(ctx context.Context) {
	w.informersFactory.Start(ctx.Done())
	w.logger.Println("waiting for cache sync")
	w.informersFactory.WaitForCacheSync(ctx.Done())

	go w.crIndexInformer.Run(ctx.Done())
	cache2.WaitForCacheSync(ctx.Done(), w.crIndexInformer.HasSynced)

	go w.depIndexInformer.Run(ctx.Done())
	cache2.WaitForCacheSync(ctx.Done(), w.depIndexInformer.HasSynced)

	w.logger.Println("started")
}
