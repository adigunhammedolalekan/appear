package k8s

import (
	"errors"
	"fmt"
	"github.com/adigunhammedolalekan/paas/build"
	"github.com/adigunhammedolalekan/paas/types"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const nameSpace = "default"
const stablePort = 6001
type K8sService interface {
	NginxDeployment(app *types.App) error
	GetService(name string) *v1.Service
	UpdateDeployment(app *types.App) error
	Logs(appName string) (string, error)
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
	if err := service.createNS(); err != nil {
		return nil, err
	}
	return service, nil
}

func (service *PaasK8sService) NginxDeployment(app *types.App) error {
	labels := map[string]string{"app" : app.Name}
	if err := service.createK8sDeployment(app.DeploymentName(),
		"nginx", labels, nil, stablePort); err != nil {
		log.Println("[k8s]: failed to create deployment for app ", app.Name, err)
		return err
	}
	if err := service.createK8sService(app.Name, labels); err != nil {
		log.Println("[K8s]: failed to create service for app ", app.Name, err)
		return err
	}
	return nil
}

func (service *PaasK8sService) createK8sService(name string,
	labels map[string]string) error {
	svc := &v1.Service{}
	svc.Name = name
	svc.Labels = labels
	svc.Namespace = nameSpace
	svc.Spec = v1.ServiceSpec{
		Type: v1.ServiceTypeNodePort,
		Selector: labels,
		Ports: []v1.ServicePort{
			{Name: "http", Protocol: "TCP", Port: stablePort},
		},
	}
	_, err := service.client.CoreV1().Services(nameSpace).Create(svc)
	if err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) createK8sDeployment(name, imageName string,
	labels map[string]string,
	envVars []build.EnvVar,
	servicePort int32) error {

	deployment := &appsv1.Deployment{}
	deployment.Name = name
	deployment.Labels = labels

	// build environment variables
	envs := make([]v1.EnvVar, 0, len(envVars))
	for _, v := range envVars {
		e := v1.EnvVar{Name: v.Key, Value: v.Value}
		envs = append(envs, e)
	}
	envs = append(envs, v1.EnvVar{Name: "PORT", Value: fmt.Sprintf("%d", servicePort)})
	container := v1.Container{}
	container.Name = fmt.Sprintf("%s-container", name)
	container.Image = imageName
	container.Ports = []v1.ContainerPort{
		{Name: "http-port", ContainerPort: servicePort},
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
		deployment, err := service.client.AppsV1().Deployments(nameSpace).Get(app.DeploymentName(), metav1.GetOptions{})
		if err != nil {
			return err
		}
		deployment.Spec.Template.Spec.Containers[0].Image = app.ImageName
		deploymentClient := service.client.AppsV1().Deployments(nameSpace)
		if _, err := deploymentClient.Update(deployment); err != nil {
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

func (service *PaasK8sService) createNS() error {
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
				log.Println(string(b[:]))
				logsString.WriteString(string(b[:]))
			}
		}
	}
	return logsString.String(), nil
}

func Int32(i int32) *int32 {
	return &i
}