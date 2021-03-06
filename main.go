package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func prefri(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	klog.Info("calling prefri")

	if expect, actual := "deployments", ar.Request.Resource.Resource; expect != actual {
		err := fmt.Errorf("unexpected resource: expect %s, actual %s", expect, actual)
		klog.Error(err)
		return toAdmissionResponse(true, nil)
	}

	weekday := time.Now().Weekday()

	if weekday == time.Friday {
		err := fmt.Errorf("Operation is prohibited")
		return toAdmissionResponse(false, err)
	}

	return toAdmissionResponse(true, nil)
}

// toAdmissionResponse is a helper function to create an AdmissionResponse
// with an embedded error
func toAdmissionResponse(allowed bool, err error) *v1beta1.AdmissionResponse {
	response := &v1beta1.AdmissionResponse{
		Allowed: allowed,
	}
	if err != nil {
		response.Result = &metav1.Status{
			Message: err.Error(),
		}
	}
	return response
}

// admitFunc is the type we use for all of our validators and mutators
type admitFunc func(v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

// serve handles the http portion of a request prior to handing to an admit
// function
func serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		klog.Errorf("contentType=%s, expect application/json", contentType)
		return
	}

	klog.V(2).Info(fmt.Sprintf("handling request: %s", body))

	// The AdmissionReview that was sent to the webhook
	requestedAdmissionReview := v1beta1.AdmissionReview{}

	// The AdmissionReview that will be returned
	responseAdmissionReview := v1beta1.AdmissionReview{}

	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(body, nil, &requestedAdmissionReview); err != nil {
		klog.Error(err)
		responseAdmissionReview.Response = toAdmissionResponse(false, err)
	} else {
		// pass to admitFunc
		responseAdmissionReview.Response = admit(requestedAdmissionReview)
	}

	// Return the same UID
	responseAdmissionReview.Response.UID = requestedAdmissionReview.Request.UID

	klog.V(2).Info(fmt.Sprintf("sending response: %v", responseAdmissionReview.Response))

	respBytes, err := json.Marshal(responseAdmissionReview)
	if err != nil {
		klog.Error(err)
	}
	if _, err := w.Write(respBytes); err != nil {
		klog.Error(err)
	}
}

func servePrefri(w http.ResponseWriter, r *http.Request) {
	serve(w, r, prefri)
}

func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")

	var config Config
	config.addFlags()
	flag.Parse()

	http.HandleFunc("/prefri", servePrefri)
	server := http.Server{
		Addr:      ":443",
		TLSConfig: configTLS(config),
	}
	server.ListenAndServeTLS("", "")
}
