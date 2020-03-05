package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestIngressCacheFullFlow(t *testing.T) {
	c := NewIngressCache()

	clusterID := "alpha"
	name := "test"
	domain := "domain.com"
	path := "/service-another"
	class := ""
	port := 8890
	ingress := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: name,
			Name:      name,
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{
					Host: domain,
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: path,
									Backend: v1beta1.IngressBackend{
										ServiceName: name,
										ServicePort: intstr.FromInt(port),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ports := map[string]map[int]string{
		name: {
			port: "http",
		},
	}

	// update with one ingress
	ing := c.Update(clusterID, ingress, ports)

	expectedIngress := &Ingress{
		Name:      name,
		Namespace: name,
		Class:     class,
		ClusterID: clusterID,
		Rules: []Rule{
			{
				Host: domain,
				RuleValue: RuleValue{
					Paths: []HTTPPath{
						{
							Path:        path + "/",
							ServiceName: name,
							ServicePort: port,
							PortName:    "http",
						},
					},
				},
			},
		},
	}

	assert.Equal(t, expectedIngress, ing)

	// retrieve updated ingress
	ings := c.GetByClass(clusterID, class)
	assert.Equal(t, 1, len(ings))

	assert.Equal(t, ing, ings[NewQualifiedName(name, name)])

	// clusterIDs should be empty because we don't have another clusters with this Qn ingress absent
	clusterIDs := c.GetClusterIDsForIngress(NewQualifiedName(name, name), class)
	assert.Equal(t, 0, len(clusterIDs))

	// check inserted service list
	services := c.GetServicesByClass(clusterID, class)
	assert.Equal(t, []QualifiedName{NewQualifiedName(name, name)}, services)

	// set service port name
	portName := "http"
	c.UpdateServicePort(clusterID, name, name, []Port{{Name: portName, Port: port}})
	updatedIngress := c.GetByClass(clusterID, class)
	assert.Equal(t, portName, updatedIngress[NewQualifiedName(name, name)].Rules[0].Paths[0].PortName)

	// remove ingress
	c.Remove(clusterID, ingress)
	currentIngresses := c.GetByClass(clusterID, class)
	assert.Equal(t, 0, len(currentIngresses))
}
