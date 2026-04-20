package k8s

var (
	kubectl string
)

func init() {
	kubectl = detectKubectl()
}
