package types

import (
	"fmt"
)

type Exposure int

const (
	ExposureNone Exposure = iota
	ExposureLoadBalancer
)

var (
	exposureNames = map[Exposure]string{
		ExposureNone:         "none",
		ExposureLoadBalancer: "loadbalancer",
	}

	exposureValues = func() map[string]Exposure {
		m := make(map[string]Exposure, len(exposureNames))
		for k, v := range exposureNames {
			m[v] = k
		}
		return m
	}()
)

func (e Exposure) String() string {
	if name, ok := exposureNames[e]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", e)
}

func (e Exposure) MarshalYAML() (interface{}, error) {
	if name, ok := exposureNames[e]; ok {
		return name, nil
	}
	return nil, fmt.Errorf("unknown exposure: %d", int(e))
}

func (e *Exposure) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	exposure, ok := exposureValues[s]
	if !ok {
		return fmt.Errorf("unknown exposure: %q", s)
	}
	*e = exposure
	return nil
}

// ToUnstructuredConfig returns the exposure configuration.
func (e *Exposure) ToUnstructuredConfig() map[string]interface{} {
	if e == nil {
		return nil
	}
	switch *e {
	case ExposureLoadBalancer:
		return map[string]interface{}{
			"loadBalancer": map[string]interface{}{
				"enabled": true,
				"port":    443,
			},
		}
	case ExposureNone:
		return map[string]interface{}{
			"nodePort": map[string]interface{}{
				"enabled": false,
			},
			"loadBalancer": map[string]interface{}{
				"enabled": false,
			},
			"route": map[string]interface{}{
				"enabled": false,
			},
		}
	default:
		return nil
	}
}
