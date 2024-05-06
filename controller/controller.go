package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	openfaas "github.com/openfaas-incubator/ingress-operator/pkg/apis/openfaas/v1alpha2"
	faasv1alpha2 "github.com/openfaas-incubator/ingress-operator/pkg/client/clientset/versioned/typed/openfaas/v1alpha2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

const (
	functionNamespaceEnv = "FUNCTION_NS"
	targetNamespaceEnv   = "TARGET_NS"
)

func main() {
	klog.SetOutput(os.Stdout)

	restConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalln("inclusterconfig failed", err)
	}

	faasClient := faasv1alpha2.NewForConfigOrDie(restConfig)
	client := kubernetes.NewForConfigOrDie(restConfig)

	startInformer(client, faasClient, getEnv(functionNamespaceEnv), getEnv(targetNamespaceEnv))
}

func startInformer(client kubernetes.Interface, faasClient *faasv1alpha2.OpenfaasV1alpha2Client, functionNamespace, targetNamespace string) {
	informer := informers.NewSharedInformerFactoryWithOptions(
		client,
		time.Second*10,
		informers.WithNamespace(functionNamespace),
	)

	stop := make(chan struct{})
	defer close(stop)

	podInformer := informer.Core().V1().Pods().Informer()
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			podObject := obj.(v1.Object)

			klog.Info("new pod:", podObject.GetName())

			fnName, ok := podObject.GetLabels()["faas_function"]
			if !ok {
				klog.Info("skipped due to no faas_function label")
				return
			}

			rpcName, ok := podObject.GetAnnotations()["grpc/faas-func-rpc"]
			if !ok {
				klog.Info("skipped because missing annotation")
				return
			}

			domainName, ok := podObject.GetAnnotations()["grpc/faas-func-domain"]
	

			createController(client, faasClient, fnName, rpcName, domainName, targetNamespace)
		},
	})

	podInformer.Run(stop)
}

func createController(client kubernetes.Interface, faasClient *faasv1alpha2.OpenfaasV1alpha2Client, fnName, rpcName, domainName, targetNamespace string) {
	path := fmt.Sprintf("/(%s)", strings.ReplaceAll(rpcName, ".", "\\."))

	ingressConfig := &openfaas.FunctionIngress{
		ObjectMeta: v1.ObjectMeta{
			Name:      fmt.Sprintf("fni-%s", fnName),
			Namespace: targetNamespace,
		},
		Spec: openfaas.FunctionIngressSpec{
			Domain:      domainName,
			Path:        path,
			Function:    fnName,
			IngressType: "nginx",
		},
	}

	klog.Info("created fni: ", fmt.Sprintf("fni-%s", fnName))

	_, err := faasClient.FunctionIngresses(targetNamespace).Create(ingressConfig)
	if err != nil {
		klog.Error("fni failed to create: ", err)
	}
}

func getEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("environment variable %s is not set", key)
	}
	return value
}