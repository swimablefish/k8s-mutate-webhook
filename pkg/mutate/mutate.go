// Package mutate deals with AdmissionReview requests and responses, it takes in the request body and returns a readily converted JSON []byte that can be
// returned from a http Handler w/o needing to further convert or modify it, it also makes testing Mutate() kind of easy w/o need for a fake http server, etc.
package mutate

import (
	"encoding/json"
	"errors"
	"fmt"
	v1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"log"
)

func mutateDaemonSetPod(clientSet *kubernetes.Clientset, pod *corev1.Pod, patch []map[string]string, verbose bool) {
	ownerReferences := pod.ObjectMeta.OwnerReferences
	if ownerReferences != nil && len(ownerReferences) > 0 {
		if verbose {
			log.Printf("owner references type: %s\n", ownerReferences[0].Kind)
		}

		if ownerReferences[0].Kind == "DaemonSet" {
			nodeName, err := getNodeNameFromPod(pod)
			if err != nil {
				log.Printf("Can't find the node the pod(%s/%s) will run, %v", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name, err)
				return
			}
			if verbose {
				log.Printf("node name: %s\n", nodeName)
				log.Printf("scheduler name: %s\n", pod.Spec.SchedulerName)
			}
			node, err := clientSet.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
			if err != nil {
				return
			}
			if val, ok := node.Labels["visenze.component"]; ok {
				// do mutation
				log.Printf("found the label, the values is %s", val)
			}
		}
	}
}

func getNodeNameFromPod(pod *corev1.Pod) (string, error) {
	// "affinity":{
	//               "nodeAffinity":{
	//                  "requiredDuringSchedulingIgnoredDuringExecution":{
	//                     "nodeSelectorTerms":[
	//                        {
	//                           "matchFields":[
	//                              {
	//                                 "key":"metadata.name",
	//                                 "operator":"In",
	//                                 "values":[
	//                                    "ip-10-0-1-156.us-west-2.compute.internal"
	//                                 ]
	//                              }
	//                           ]
	//                        }
	//                     ]
	//                  }

	for _, t := range pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		for _, f := range t.MatchFields {
			if f.Key == "metadata.name" {
				return f.Values[0], nil
			}
		}
	}

	return "", errors.New("can't find the node for the ds pod")
}

// Mutate mutates
func Mutate(body []byte, verbose bool, clientSet *kubernetes.Clientset) ([]byte, error) {
	if verbose {
		log.Printf("recv: %s\n", string(body)) // untested section
	}

	// unmarshal request into AdmissionReview struct
	admReview := v1beta1.AdmissionReview{}
	if err := json.Unmarshal(body, &admReview); err != nil {
		return nil, fmt.Errorf("unmarshaling request failed with %s", err)
	}

	var err error
	var pod *corev1.Pod

	responseBody := []byte{}
	ar := admReview.Request
	resp := v1beta1.AdmissionResponse{}

	if ar != nil {

		// get the Pod object and unmarshal it into its struct, if we cannot, we might as well stop here
		if err := json.Unmarshal(ar.Object.Raw, &pod); err != nil {
			return nil, fmt.Errorf("unable unmarshal pod json object %v", err)
		}
		// set response options
		resp.Allowed = true
		resp.UID = ar.UID
		pT := v1beta1.PatchTypeJSONPatch
		resp.PatchType = &pT // it's annoying that this needs to be a pointer as you cannot give a pointer to a constant?

		// the actual mutation is done by a string in JSONPatch style, i.e. we don't _actually_ modify the object, but
		// tell K8S how it should modifiy it
		p := []map[string]string{}
		mutateDaemonSetPod(clientSet, pod, p, verbose)
		log.Printf("%v", p)
		//for i := range pod.Spec.Containers {
		//	patch := map[string]string{
		//		"op":    "replace",
		//		"path":  fmt.Sprintf("/spec/containers/%d/image", i),
		//		"value": "debian",
		//	}
		//	p = append(p, patch)
		//}
		// parse the []map into JSON
		resp.Patch, err = json.Marshal(p)

		// Success, of course ;)
		resp.Result = &metav1.Status{
			Status: "Success",
		}

		admReview.Response = &resp
		// back into JSON so we can return the finished AdmissionReview w/ Response directly
		// w/o needing to convert things in the http handler
		responseBody, err = json.Marshal(admReview)
		if err != nil {
			return nil, err // untested section
		}
	}

	if verbose {
		log.Printf("resp: %s\n", string(responseBody)) // untested section
	}

	return responseBody, nil
}
