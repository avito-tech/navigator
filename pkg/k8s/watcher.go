package k8s

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
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
	ingressNotifier
	gatewayNotifier
}

type eventsCounter interface {
	EventsCount() int32
}

type Watcher interface {
	Start(ctx context.Context)
}

type watcher struct {
	logger           logrus.FieldLogger
	informersFactory informers.SharedInformerFactory
	crIndexInformer  cache2.SharedIndexInformer
	depIndexInformer cache2.SharedIndexInformer
	gwIndexInformer  cache2.SharedIndexInformer

	ingressCounter       eventsCounter
	serviceCounter       eventsCounter
	endpointCounter      eventsCounter
	gatewayCounter       eventsCounter
	canaryReleaseCounter eventsCounter
	nexusCounter         eventsCounter
}

func NewWatcher(
	logger logrus.FieldLogger,
	cs kubernetes.Interface,
	extCs versioned.Interface,
	k8sCache Cache,
	canary Canary,
	notifier notifier,
	nexusCache NexusCache,
	ingressCache IngressCache,
	gatewayCache GatewayCache,
	clusterID string,
) Watcher {

	logger = logger.WithField("cluster", clusterID)
	informersFactory := informers.NewSharedInformerFactory(cs, ResyncTimeout)
	canaryReleaseInf := navigatorInformer.NewCanaryReleaseInformer(extCs, "", ResyncTimeout, nil)
	nexusInf := navigatorInformer.NewNexusInformer(extCs, "", ResyncTimeout, nil)
	gatewayInf := navigatorInformer.NewGatewayInformer(extCs, "", ResyncTimeout, nil)

	serviceEventHandler := NewServiceEventHandler(
		logger,
		k8sCache,
		ingressCache,
		notifier,
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

	ingressEventHandler := NewIngressEventHandler(
		logger,
		nexusCache,
		ingressCache,
		gatewayCache,
		k8sCache,
		notifier,
		notifier,
		clusterID,
	)

	gatewayEventHandler := NewGatewayEventHandler(
		logger,
		ingressCache,
		nexusCache,
		gatewayCache,
		notifier,
		notifier,
		clusterID,
	)

	epLister := informersFactory.Core().V1().Endpoints().Lister()
	canaryReleaseEventHandler := NewCanaryReleaseEventHandler(logger, k8sCache, canary, clusterID, epLister, endpointEventHandler)
	nexusHandler := NewNexusEventHandler(logger, nexusCache, notifier, clusterID)

	informersFactory.Core().V1().Services().Informer().AddEventHandler(serviceEventHandler)
	informersFactory.Core().V1().Endpoints().Informer().AddEventHandler(endpointEventHandler)
	informersFactory.Extensions().V1beta1().Ingresses().Informer().AddEventHandler(ingressEventHandler)

	canaryReleaseInf.AddEventHandler(canaryReleaseEventHandler)
	nexusInf.AddEventHandler(nexusHandler)
	gatewayInf.AddEventHandler(gatewayEventHandler)

	return &watcher{
		logger:           logger.WithField("context", "k8s.Watcher"),
		informersFactory: informersFactory,
		crIndexInformer:  canaryReleaseInf,
		depIndexInformer: nexusInf,
		gwIndexInformer:  gatewayInf,

		serviceCounter:       serviceEventHandler,
		endpointCounter:      endpointEventHandler,
		ingressCounter:       ingressEventHandler,
		gatewayCounter:       gatewayEventHandler,
		canaryReleaseCounter: canaryReleaseEventHandler,
		nexusCounter:         nexusHandler,
	}
}

func (w *watcher) Start(ctx context.Context) {
	startTime := time.Now()
	w.logger.Infoln("waiting for cache sync")
	w.informersFactory.Start(ctx.Done())
	w.informersFactory.WaitForCacheSync(ctx.Done())

	go w.crIndexInformer.Run(ctx.Done())
	go w.depIndexInformer.Run(ctx.Done())
	go w.gwIndexInformer.Run(ctx.Done())

	cache2.WaitForCacheSync(
		ctx.Done(),
		w.crIndexInformer.HasSynced,
		w.depIndexInformer.HasSynced,
		w.gwIndexInformer.HasSynced,
	)

	services, _ := w.informersFactory.Core().V1().Services().Lister().List(labels.Everything())
	endpoints, _ := w.informersFactory.Core().V1().Endpoints().Lister().List(labels.Everything())
	ingresses, _ := w.informersFactory.Extensions().V1beta1().Ingresses().Lister().List(labels.Everything())
	canaries := w.crIndexInformer.GetIndexer().List()
	nexuses := w.depIndexInformer.GetIndexer().List()
	gateways := w.gwIndexInformer.GetIndexer().List()

	syncers := []cache2.InformerSynced{
		makeSyncer(w.serviceCounter, int32(len(services))),
		makeSyncer(w.endpointCounter, int32(len(endpoints))),
		makeSyncer(w.ingressCounter, int32(len(ingresses))),
		makeSyncer(w.canaryReleaseCounter, int32(len(canaries))),
		makeSyncer(w.nexusCounter, int32(len(nexuses))),
		makeSyncer(w.gatewayCounter, int32(len(gateways))),
	}
	cache2.WaitForCacheSync(ctx.Done(), syncers...)

	syncedTime := time.Now()
	delay := syncedTime.Sub(startTime)

	l := w.logger.WithField("delay", delay.String())
	if delay > time.Duration(time.Second*30) {
		l.Warnln("cache synced")
	} else {
		l.Infoln("cache synced")
	}
}

func makeSyncer(c eventsCounter, needed int32) cache2.InformerSynced {
	return func() bool { return c.EventsCount() >= needed }
}
