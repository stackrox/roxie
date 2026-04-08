package component

import "fmt"

type Component int

const (
	All Component = iota
	Central
	SecuredCluster
	Operator
)

func FromArgs(args []string) (Component, error) {
	if len(args) > 0 {
		switch args[0] {
		case "both", "all":
			return All, nil
		case "central":
			return Central, nil
		case "secured-cluster", "sensor":
			return SecuredCluster, nil
		case "operator":
			return Operator, nil
		default:
			return All, fmt.Errorf("unknown component: %s", args[0])
		}
	}
	return All, nil
}

func (c Component) String() string {
	switch c {
	case All:
		return "Central and Secured Cluster"
	case Central:
		return "Central"
	case SecuredCluster:
		return "Secured Cluster"
	case Operator:
		return "Operator"
	default:
		return fmt.Sprintf("Unknown(%d)", c)
	}
}

func (c Component) IncludesCentral() bool {
	if c == SecuredCluster || c == Operator {
		return false
	}
	return true
}

func (c Component) IncludesSensor() bool {
	if c == Central || c == Operator {
		return false
	}
	return true
}

func (c Component) IncludesOperator() bool {
	return c == Operator || c == All
}

func (c Component) IncludesOperatorExplicitly() bool {
	return c == Operator
}

func (c Component) IncludesBothCentralAndSensor() bool {
	return c == Central || c == SecuredCluster || c == All
}
