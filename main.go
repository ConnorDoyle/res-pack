package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/comail/colog"
	"github.com/julienschmidt/httprouter"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	schedulerapi "k8s.io/kubernetes/plugin/pkg/scheduler/api"
)

const (
	versionPath      = "/version"
	apiPrefix        = "/scheduler"
	bindPath         = apiPrefix + "/bind"
	predicatesPrefix = apiPrefix + "/predicates"
	prioritiesPrefix = apiPrefix + "/prioritize"

	// TODO(CD): parameterize this
	scarceResource = "intel.com/foo"
)

var (
	version string // injected via ldflags at build time

	NoFilter = Predicate{
		Name: "unsupported",
		Func: func(pod v1.Pod, node v1.Node) (bool, error) {
			return true, fmt.Errorf("This extender doesn't support Filter.  Please make 'FilterVerb' be empty in your ExtenderConfig.")
		},
	}

	// If the pod in question does not request any of the resource to pack,
	// then score each node equally (0).
	//
	// Otherwise, score nodes based on the amount of scarce resource.
	// The nodes with the least available should get the highest score.
	// The effect is to prefer to bin-pack pods that request scarce resources
	// (i.e. worst-fit) and reduce fragmentation.
	ResPriority = Prioritize{
		Name: "res-pack",
		Func: func(pod v1.Pod, nodes []v1.Node) (*schedulerapi.HostPriorityList, error) {
			var priorityList schedulerapi.HostPriorityList
			priorityList = make([]schedulerapi.HostPriority, len(nodes))

			// If the pod does not request any `scarceResource`, apply no priority.
			if !podRequestsResource(pod, scarceResource) {
				for i, node := range nodes {
					priorityList[i] = schedulerapi.HostPriority{
						Host:  node.Name,
						Score: 0,
					}
				}
				return &priorityList, nil
			}

			// Set priority based on partially consumed scarce resources.
			for i, node := range nodes {
				priorityList[i] = schedulerapi.HostPriority{
					Host:  node.Name,
					Score: 0,
				}
			}
			return &priorityList, nil
		},
	}

	NoBind = Bind{
		Func: func(podName string, podNamespace string, podUID types.UID, node string) error {
			return fmt.Errorf("This extender doesn't support Bind.  Please make 'BindVerb' be empty in your ExtenderConfig.")
		},
	}
)

func podRequestsResource(pod v1.Pod, resource string) bool {
	containerRequestsResource := func(container v1.Container) bool {
		for resName, quantity := range container.Resources.Requests {
			if string(resName) == resource && quantity.MilliValue() > 0 {
				return true
			}
		}
		for resName, quantity := range container.Resources.Limits {
			if string(resName) == resource && quantity.MilliValue() > 0 {
				return true
			}
		}
		return false
	}

	for _, c := range pod.Spec.InitContainers {
		if containerRequestsResource(c) {
			return true
		}
	}
	for _, c := range pod.Spec.Containers {
		if containerRequestsResource(c) {
			return true
		}
	}
	return false
}

func StringToLevel(levelStr string) colog.Level {
	switch level := strings.ToUpper(levelStr); level {
	case "TRACE":
		return colog.LTrace
	case "DEBUG":
		return colog.LDebug
	case "INFO":
		return colog.LInfo
	case "WARNING":
		return colog.LWarning
	case "ERROR":
		return colog.LError
	case "ALERT":
		return colog.LAlert
	default:
		log.Printf("warning: LOG_LEVEL=\"%s\" is empty or invalid, fallling back to \"INFO\".\n", level)
		return colog.LInfo
	}
}

func main() {
	colog.SetDefaultLevel(colog.LInfo)
	colog.SetMinLevel(colog.LInfo)
	colog.SetFormatter(&colog.StdFormatter{
		Colors: true,
		Flag:   log.Ldate | log.Ltime | log.Lshortfile,
	})
	colog.Register()
	level := StringToLevel(os.Getenv("LOG_LEVEL"))
	log.Print("Log level was set to ", strings.ToUpper(level.String()))
	colog.SetMinLevel(level)

	router := httprouter.New()
	AddVersion(router)

	AddPredicate(router, NoFilter)

	priorities := []Prioritize{ResPriority}
	for _, p := range priorities {
		AddPrioritize(router, p)
	}

	AddBind(router, NoBind)

	log.Print("info: server starting on the port :80")
	if err := http.ListenAndServe(":80", router); err != nil {
		log.Fatal(err)
	}
}
