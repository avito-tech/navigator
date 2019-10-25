package resources

import "strings"

const (
	DefaultDynamicClusterName = "navigator"

	healthCheckNamePrefix = "~~"
)

type FuncOpt func(resource interface{})

type DynamicConfiger interface {
	SetClusterName(name string)
}

type DynamicConfig struct {
	DynamicClusterName string
}

func (c *DynamicConfig) SetClusterName(name string) {
	c.DynamicClusterName = name
}

func WithClusterName(name string) FuncOpt {
	return func(resource interface{}) {
		r, ok := resource.(DynamicConfiger)
		if !ok {
			return
		}
		r.SetClusterName(name)
	}
}

type HealthCheckConfiger interface {
	SetHealthCheck(path string, port int)
}

type HealthCheckConfig struct {
	EnableHealthCheck bool
	HealthCheckName   string
	HealthCheckPath   string
	HealthCheckPort   int
}

func (c *HealthCheckConfig) SetHealthCheck(path string, port int) {

	c.EnableHealthCheck = true
	c.HealthCheckPort = port

	if strings.HasPrefix(path, "/") {
		c.HealthCheckPath = path
		c.HealthCheckName = healthCheckNamePrefix + strings.TrimPrefix(path, "/")
	} else {
		c.HealthCheckPath = "/" + path
		c.HealthCheckName = healthCheckNamePrefix + path
	}
}

func WithHealthCheck(path string, port int) FuncOpt {
	return func(resource interface{}) {
		r, ok := resource.(HealthCheckConfiger)
		if !ok {
			return
		}
		r.SetHealthCheck(path, port)
	}
}

func SetOpts(resource interface{}, opts ...FuncOpt) {
	for _, opt := range opts {
		opt(resource)
	}
}
