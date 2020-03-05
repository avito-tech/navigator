package k8s

import (
	"strings"
	"sync"

	"k8s.io/api/extensions/v1beta1"
)

const (
	IngressClassAnnotation  = "kubernetes.io/ingress.class"
	RewriteTargetAnnotation = "nginx.ingress.kubernetes.io/rewrite-target"
	CookieAffinity          = "navigator.avito.ru/cookie-affinity"
)

type IngressCache interface {
	Update(clusterID string, ingress *v1beta1.Ingress, ports map[string]map[int]string) *Ingress
	Remove(clusterID string, ingress *v1beta1.Ingress) *Ingress
	GetByClass(clusterID string, class string) map[QualifiedName]*Ingress
	GetServicesByClass(clusterID string, class string) []QualifiedName
	GetClusterIDsForIngress(qn QualifiedName, class string) []string
	UpdateServicePort(clusterID string, namespace string, name string, ports []Port) []*Ingress
}

// Ingress incapsulate kube Ingress entity
type Ingress struct {
	Name, Namespace string
	Annotations     map[string]string
	Class           string
	ClusterID       string
	Rules           []Rule
}

func (i *Ingress) copy() *Ingress {
	rules := make([]Rule, 0, len(i.Rules))
	for _, r := range i.Rules {
		rules = append(rules, r.copy())
	}
	annotations := make(map[string]string, len(i.Annotations))
	for k, v := range i.Annotations {
		annotations[k] = v
	}
	return &Ingress{
		Name:        i.Name,
		Namespace:   i.Namespace,
		Annotations: annotations,
		Class:       i.Class,
		ClusterID:   i.ClusterID,
		Rules:       rules,
	}
}

type Rule struct {
	Host string
	RuleValue
}

func (r *Rule) copy() Rule {
	rulePaths := make([]HTTPPath, 0, len(r.Paths))
	for _, p := range r.Paths {
		rulePaths = append(rulePaths, p.copy())
	}
	return Rule{
		Host: r.Host,
		RuleValue: RuleValue{
			Paths: rulePaths,
		},
	}
}

type RuleValue struct {
	Paths []HTTPPath
}

type HTTPPath struct {
	Path           string
	ServiceName    string
	ServicePort    int
	Rewrite        string
	CookieAffinity *string
	// evaluated from Service kube entity
	PortName string
}

func (p *HTTPPath) copy() HTTPPath {
	var cookieAffinity *string
	if p.CookieAffinity != nil {
		s := *p.CookieAffinity
		cookieAffinity = &s
	}
	return HTTPPath{
		Path:           p.Path,
		ServiceName:    p.ServiceName,
		ServicePort:    p.ServicePort,
		Rewrite:        p.Rewrite,
		CookieAffinity: cookieAffinity,
		PortName:       p.PortName,
	}
}

type ingressCache struct {
	// cluster ID -> ingress class -> qualified name -> Ingress resource
	cache map[string]map[string]map[QualifiedName]*Ingress

	// cluster ID -> service Qn -> ingress Qn -> Ingress
	serviceToIngress map[string]map[QualifiedName]map[QualifiedName]*Ingress

	mu sync.RWMutex
}

func NewIngressCache() IngressCache {
	return &ingressCache{
		cache:            make(map[string]map[string]map[QualifiedName]*Ingress),
		serviceToIngress: make(map[string]map[QualifiedName]map[QualifiedName]*Ingress),
	}
}

func (i *ingressCache) Update(clusterID string, ingress *v1beta1.Ingress, ports map[string]map[int]string) *Ingress {
	// obtain ingress class (in case of absence we use empty class "")
	class := ingress.Annotations[IngressClassAnnotation]
	i.mu.Lock()
	if _, ok := i.cache[clusterID]; !ok {
		i.cache[clusterID] = make(map[string]map[QualifiedName]*Ingress)
	}
	if _, ok := i.cache[clusterID][class]; !ok {
		i.cache[clusterID][class] = make(map[QualifiedName]*Ingress)
	}

	if _, ok := i.serviceToIngress[clusterID]; !ok {
		i.serviceToIngress[clusterID] = make(map[QualifiedName]map[QualifiedName]*Ingress)
	}

	// compute parameters out of annotations:
	rewrite := ingress.Annotations[RewriteTargetAnnotation]
	var cookieAffinity *string
	if c, ok := ingress.Annotations[CookieAffinity]; ok {
		cookieAffinity = &c
	}

	var rules []Rule
	var usedServices []QualifiedName

	for _, r := range ingress.Spec.Rules {
		var paths []HTTPPath
		for _, p := range r.HTTP.Paths {
			path := p.Path
			if !strings.HasSuffix(path, "/") {
				path += "/"
			}
			portName := ""
			if _, ok := ports[p.Backend.ServiceName]; ok {
				portName = ports[p.Backend.ServiceName][p.Backend.ServicePort.IntValue()]
			}
			paths = append(paths, HTTPPath{
				Path:           path,
				ServiceName:    p.Backend.ServiceName,
				ServicePort:    p.Backend.ServicePort.IntValue(),
				PortName:       portName,
				Rewrite:        rewrite,
				CookieAffinity: cookieAffinity,
			})
			if _, ok := i.serviceToIngress[clusterID][NewQualifiedName(ingress.Namespace, p.Backend.ServiceName)]; !ok {
				i.serviceToIngress[clusterID][NewQualifiedName(ingress.Namespace, p.Backend.ServiceName)] = make(map[QualifiedName]*Ingress)
			}
			usedServices = append(usedServices, NewQualifiedName(ingress.Namespace, p.Backend.ServiceName))
		}
		rule := Rule{
			Host: r.Host,
			RuleValue: RuleValue{
				Paths: paths,
			},
		}

		rules = append(rules, rule)
	}

	ing := &Ingress{
		Name:        ingress.Name,
		Namespace:   ingress.Namespace,
		Annotations: ingress.Annotations,
		Class:       class,
		ClusterID:   clusterID,
		Rules:       rules,
	}

	i.cache[clusterID][class][NewQualifiedName(ingress.Namespace, ingress.Name)] = ing
	for _, svcQn := range usedServices {
		i.serviceToIngress[clusterID][svcQn][NewQualifiedName(ingress.Namespace, ingress.Name)] = ing
	}

	i.mu.Unlock()

	return ing
}

func (i *ingressCache) Remove(clusterID string, ingress *v1beta1.Ingress) *Ingress {
	class := ingress.Annotations[IngressClassAnnotation]
	i.mu.Lock()
	ing := i.cache[clusterID][class][NewQualifiedName(ingress.Namespace, ingress.Name)]
	delete(i.cache[clusterID][class], NewQualifiedName(ingress.Namespace, ingress.Name))

	for _, r := range ingress.Spec.Rules {
		for _, p := range r.HTTP.Paths {
			delete(i.serviceToIngress[clusterID][NewQualifiedName(ingress.Namespace, p.Backend.ServiceName)], NewQualifiedName(ingress.Namespace, ingress.Name))
		}
	}
	i.mu.Unlock()
	return ing
}

func (i *ingressCache) GetByClass(clusterID string, class string) map[QualifiedName]*Ingress {
	// firstly, get merged ingresses by qualified name in other clusters
	i.mu.RLock()
	defer i.mu.RUnlock()

	return i.getByClass(clusterID, class)
}

func (i *ingressCache) getByClass(clusterID string, class string) map[QualifiedName]*Ingress {
	// firstly, get merged ingresses by qualified name in other clusters
	ings := make(map[QualifiedName]*Ingress)
	for cID := range i.cache {
		if cID == clusterID {
			continue
		}
		for c, ingresses := range i.cache[cID] {
			if c == class {
				for _, ing := range ingresses {
					ings[NewQualifiedName(ing.Namespace, ing.Name)] = ing.copy()
				}
			}
		}
	}
	if localIngresses, ok := i.cache[clusterID]; ok {
		if ingresses, ok := localIngresses[class]; ok {
			for _, ing := range ingresses {
				ings[NewQualifiedName(ing.Namespace, ing.Name)] = ing.copy()
			}
		}
	}
	return ings
}

// GetClusterIDsForIngress returns cluster IDs which are affected by given kube ingress filtered by ingress class
// (actually, clusters where we don't have ingress with such qualified name)
func (i *ingressCache) GetClusterIDsForIngress(qn QualifiedName, class string) []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	var clusterIDs []string
	for clusterID, ingresses := range i.cache {
		if ings, ok := ingresses[class]; ok {
			if _, ok := ings[qn]; !ok {
				clusterIDs = append(clusterIDs, clusterID)
			}
		}
	}

	return clusterIDs
}

func (i *ingressCache) GetServicesByClass(clusterID string, class string) []QualifiedName {
	services := make(map[QualifiedName]struct{})
	i.mu.RLock()
	defer i.mu.RUnlock()
	ingresses := i.getByClass(clusterID, class)
	for _, ing := range ingresses {
		for _, rule := range ing.Rules {
			for _, path := range rule.Paths {
				services[NewQualifiedName(ing.Namespace, path.ServiceName)] = struct{}{}
			}
		}
	}

	var qns []QualifiedName
	for qn := range services {
		qns = append(qns, qn)
	}

	return qns
}

func (i *ingressCache) UpdateServicePort(clusterID string, namespace string, name string, ports []Port) []*Ingress {
	i.mu.Lock()
	defer i.mu.Unlock()
	var updatedIngresses []*Ingress
	updatedIngressesMap := i.serviceToIngress[clusterID][NewQualifiedName(namespace, name)]
	for _, ing := range updatedIngressesMap {
		for _, r := range ing.Rules {
			for pathIndex, path := range r.Paths {
				for _, port := range ports {
					if path.ServicePort == port.Port {
						r.Paths[pathIndex].PortName = port.Name
						updatedIngresses = append(updatedIngresses, ing)
					}
				}
			}
		}
	}

	return updatedIngresses
}
