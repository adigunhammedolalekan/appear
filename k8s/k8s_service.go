package k8s

import(
	"errors"
	"fmt"
	"github.com/adigunhammedolalekan/paas/docker"
	"github.com/adigunhammedolalekan/paas/types"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const nameSpace = "appear-namespace"
const stablePort = 6003

type K8sService interface {
	NginxDeployment(app *types.App) error
	GetService(name string) *v1.Service
	UpdateDeployment(app *types.App) error
	Logs(appName string) (string, error)
	ScaleApp(deploymentName string, replica int32) error
	GetPodNode(podName string) (*v1.Node, error)
}

type PaasK8sService struct {
	client *kubernetes.Clientset
}

func NewK8sService() (K8sService, error) {
	configPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	service := &PaasK8sService{client: client}
	if err := service.createNameSpaceIfNotExists(); err != nil {
		return nil, err
	}
	return service, nil
}

// NginxDeployment is the default nginx image that'll be deployed
// when a new app is created
func (service *PaasK8sService) NginxDeployment(app *types.App) error {
	labels := map[string]string{"app": app.Name}
	if err := service.createK8sDeployment(app.DeploymentName(),
		"nginx", labels, nil, stablePort); err != nil {
		log.Println("[k8s]: failed to create deployment for app ", app.Name, err)
		return err
	}
	if err := service.createK8sService(app.Name, labels, 80, 0); err != nil {
		log.Println("[K8s]: failed to create service for app ", app.Name, err)
		return err
	}
	if err := service.createIngressForService(service.GetService(app.Name), 80); err != nil {
		log.Println("failed to create Ingress for service: ", err)
	}
	return nil
}

func (service *PaasK8sService) createK8sService(name string,
	labels map[string]string, servicePort, nodePort int32) error {
	svc := &v1.Service{}
	svc.Name = name
	svc.Labels = labels
	svc.Namespace = nameSpace
	port := v1.ServicePort{Name: "http", Protocol: "TCP", Port: servicePort}
	if nodePort != 0 {
		port.NodePort = nodePort
	}
	ports := []v1.ServicePort{port}
	svc.Spec = v1.ServiceSpec{
		Type:     v1.ServiceTypeNodePort,
		Selector: labels,
		Ports:    ports,
	}
	_, err := service.client.CoreV1().Services(nameSpace).Create(svc)
	if err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) createK8sDeployment(name, imageName string,
	labels map[string]string,
	envVars []docker.EnvVar, port int32) error {

	deployment := &appsv1.Deployment{}
	deployment.Name = name
	deployment.Labels = labels

	// docker environment variables
	envs := make([]v1.EnvVar, 0, len(envVars))
	for _, v := range envVars {
		e := v1.EnvVar{Name: v.Key, Value: v.Value}
		envs = append(envs, e)
	}
	envs = append(envs, v1.EnvVar{Name: "PORT", Value: fmt.Sprintf("%d", port)})
	container := v1.Container{}
	container.Name = fmt.Sprintf("%s-container", name)
	container.Image = imageName
	container.Ports = []v1.ContainerPort{
		{Name: "http-port", ContainerPort: port},
	}
	container.Env = envs
	container.ImagePullPolicy = v1.PullAlways
	podTemplate := v1.PodTemplateSpec{}
	podTemplate.Labels = labels
	podTemplate.Name = name
	podTemplate.Spec = v1.PodSpec{
		Containers: []v1.Container{
			container,
		},
	}
	deployment.Spec = appsv1.DeploymentSpec{
		Replicas: Int32(1),
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: podTemplate,
	}
	_, err := service.client.AppsV1().Deployments(nameSpace).Create(deployment)
	if err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) GetService(name string) *v1.Service {
	svc, err := service.client.CoreV1().Services(nameSpace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	return svc
}

func (service *PaasK8sService) UpdateDeployment(app *types.App) error {
	svc := service.GetService(app.Name)
	if svc == nil {
		return errors.New("failed to find deployment service")
	}

	log.Println("Updating deployment for ", app.ImageName)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		oldPort := svc.Spec.Ports[0].NodePort
		if err := service.client.CoreV1().Services(nameSpace).Delete(svc.Name,
			&metav1.DeleteOptions{}); err != nil {
			log.Println("failed to delete service ", err)
			return err
		}
		labels := map[string]string{"app": app.Name}
		if err := service.createK8sService(app.Name, labels, stablePort, oldPort); err != nil {
			log.Println("failed to create new service for updated app: ", err)
			return err
		}
		if err := service.deleteIngress(svc.Name); err != nil {
			log.Println("failed to delete service Ingress ", err)
			return err
		}
		if err := service.createIngressForService(svc, stablePort); err != nil {
			log.Println("failed to create ingress for service ", err)
			return err
		}
		deployment, err := service.client.AppsV1().Deployments(nameSpace).Get(app.DeploymentName(), metav1.GetOptions{})
		if err != nil {
			log.Println("failed to get deployment object ", err)
			return err
		}
		deployment.Spec.Template.Spec.Containers[0].Image = app.ImageName
		deploymentClient := service.client.AppsV1().Deployments(nameSpace)
		if _, err := deploymentClient.Update(deployment); err != nil {
			log.Println("failed to update deployment ", err)
			return err
		}
		return nil
	})
	if err != nil {
		log.Println("failed to update deployment ", err)
		return err
	}
	return nil
}

func (service *PaasK8sService) createNameSpaceIfNotExists() error {
	ns := &v1.Namespace{}
	ns.Name = nameSpace
	_, err := service.client.CoreV1().Namespaces().Create(ns)
	if err != nil {
		log.Println("failed to create nameSpace ", err)
	}
	return nil
}

func (service *PaasK8sService) Logs(appName string) (string, error) {
	pods, err := service.client.CoreV1().Pods(nameSpace).List(metav1.ListOptions{})
	if err != nil {
		log.Println("Pods log ", err)
		return "", err
	}
	logsString := &strings.Builder{}
	for _, v := range pods.Items {
		if strings.HasPrefix(v.Name, appName) {
			logs := service.client.CoreV1().Pods(nameSpace).GetLogs(v.Name, &v1.PodLogOptions{})
			r, err := logs.Stream()
			if err != nil {
				log.Println("pod.Stream() error ", err)
				return "", err
			}
			for {
				b := make([]byte, 512)
				_, err := r.Read(b)
				if err != nil {
					break
				}
				logsString.WriteString(string(b[:]))
				log.Println(string(b[:]))
			}
		}
	}
	return logsString.String(), nil
}

func (service *PaasK8sService) ScaleApp(deploymentName string, replica int32) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		deploymentClient := service.client.AppsV1().Deployments(nameSpace)
		deployment, err := deploymentClient.Get(deploymentName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		deployment.Spec.Replicas = Int32(replica)
		if _, err := deploymentClient.Update(deployment); err != nil {
			return err
		}
		return nil
	})
}

func (service *PaasK8sService) UpdateEnvironmentVars(app *types.App, envs []docker.EnvVar) error {
	c := service.client.AppsV1().Deployments(nameSpace)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		deployment, err := c.Get(app.DeploymentName(), metav1.GetOptions{})
		if err != nil {
			return err
		}
		for _, v := range envs {
			deployment.Spec.Template.Spec.Containers[0].Env = append(
				deployment.Spec.Template.Spec.Containers[0].Env, v1.EnvVar{Name: v.Key, Value: v.Value},
			)
		}
		if _, err := c.Update(deployment); err != nil {
			return err
		}
		return nil
	})
}

func (service *PaasK8sService) createIngressForService(svc *v1.Service, port int32) error {
	if svc == nil {
		return errors.New("target service not found")
	}
	name := fmt.Sprintf("%s-ingress", svc.Name)
	ingress := &extensions.Ingress{}
	// add re-write anotations
	// so that app can be accessed from the same host
	// kubernetes.io/ingress.class: "nginx"
	// nginx.ingress.kubernetes.io/rewrite-target: /$1
	ingress.Annotations = map[string]string{
		"kubernetes.io/ingress.class" : "nginx",
		"nginx.ingress.kubernetes.io/rewrite-target" : "/$1",
	}
	ingress.Name = name
	backend := &extensions.IngressBackend{
		ServiceName: svc.Name,
		ServicePort: intstr.IntOrString{IntVal: port},
	}
	ingress.Spec.Backend = backend
	ingressRule := extensions.IngressRule{}
	ingressRule.HTTP = &extensions.HTTPIngressRuleValue{}
	ingressRule.HTTP.Paths = []extensions.HTTPIngressPath{
		{Path: fmt.Sprintf("/%s/?(.*)", svc.Name), Backend: *backend},
	}
	ingress.Spec.Rules = []extensions.IngressRule{
		ingressRule,
	}
	if _, err := service.client.ExtensionsV1beta1().Ingresses(nameSpace).Create(ingress); err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) deleteIngress(name string) error {
	client := service.client.ExtensionsV1beta1().Ingresses(nameSpace)
	ingressName := fmt.Sprintf("%s-ingress", name)
	if err := client.Delete(ingressName, &metav1.DeleteOptions{}); err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) createDatabaseLbService(name string, port int32) error {
	svc := &v1.Service{}
	labels := map[string]string{"database" : name}
	svc.Name = name
	svc.Labels = labels
	svc.Spec = v1.ServiceSpec{
		Selector: labels,
		Type: v1.ServiceTypeLoadBalancer,
		Ports: []v1.ServicePort{{Name: "db-port", Protocol: "TCP", Port: port}},
	}
	if _, err := service.client.CoreV1().Services(nameSpace).Create(svc); err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) CreateDatabaseStatefulSet(name, imageName string) error {
	serviceName := fmt.Sprintf("%s-service", name)
	labels := map[string]string{"database" : name}
	statefullSetClient := service.client.AppsV1().StatefulSets(nameSpace)
	template := v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			TerminationGracePeriodSeconds: Int64(10),
			Containers: []v1.Container{
				{Image: imageName},
			},
		},
	}
	statefulSet := &appsv1.StatefulSet{}
	statefulSet.Name = fmt.Sprintf("%s-statefulset", name)
	statefulSet.Labels = labels
	statefulSet.Spec = appsv1.StatefulSetSpec{
		Replicas: Int32(1),
		ServiceName: serviceName,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: template,
	}
	if _, err := statefullSetClient.Create(statefulSet); err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) GetPodNode(podName string) (*v1.Node, error) {
	pods, err := service.client.CoreV1().Pods(nameSpace).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, p := range pods.Items {
		if strings.HasPrefix(p.Name, podName) {
			return service.getNode(p.Spec.NodeName)
		}
	}
	return nil, errors.New("node not found")
}

func (service *PaasK8sService) getNode(nodeName string) (*v1.Node, error) {
	node, err := service.client.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (service *PaasK8sService) GetIngress(name string) (*extensions.Ingress, error) {
	ingress, err := service.client.ExtensionsV1beta1().Ingresses(nameSpace).Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return ingress, nil
}

func Int32(i int32) *int32 {
	return &i
}
func String(s string) *string {
	return &s
}
func Int64(i int64) *int64 {
	return &i
}
