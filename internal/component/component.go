package component

import "fmt"

type Component int

const (
	Both Component = iota
	All
	Central
	SecuredCluster
	Operator
)

func FromArgs(args []string) (Component, error) {
	if len(args) > 0 {
		switch args[0] {
		case "both":
			return Both, nil
		case "all":
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
	case Both:
		return "Central and Secured Cluster"
	case All:
		return "Central, Secured Cluster, and Operator"
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
	return c == Both || c == All || c == Central
}

func (c Component) IncludesSensor() bool {
	return c == Both || c == All || c == SecuredCluster
}

func (c Component) IncludesOperator() bool {
	return c == All || c == Operator
}

func (c Component) IncludesOperatorExplicitly() bool {
	return c == Operator
}

func (c Component) IncludesBothCentralAndSensor() bool {
	return c == Both || c == All
}
