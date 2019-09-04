package k8s

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	cfg "github.com/adigunhammedolalekan/paas/config"
	"github.com/adigunhammedolalekan/paas/docker"
	"github.com/adigunhammedolalekan/paas/types"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
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
const secretName = "appear-registry-secret"

type K8sService interface {
	NginxDeployment(app *types.App) error
	GetService(name string) *v1.Service
	UpdateDeployment(app *types.App) error
	Logs(appName string) (string, error)
	ScaleApp(deploymentName string, replica int32) error
	GetPodNode(podName string) (*v1.Node, error)
}

type PaasK8sService struct {
	client   *kubernetes.Clientset
	registry *cfg.Registry
}

func NewK8sService(registry *cfg.Registry) (K8sService, error) {
	configPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	service := &PaasK8sService{client: client, registry: registry}
	if err := service.createNameSpaceIfNotExists(); err != nil {
		return nil, err
	}
	if err := service.createRegistrySecret(); err != nil {
		log.Println("WARNING: cannot create private registry secret: ", err)
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

func (service *PaasK8sService) createRegistrySecret() error {
	secret := &v1.Secret{}
	secret.Name = secretName
	secret.Type = v1.SecretTypeDockerConfigJson
	data, err := service.dockerConfigJson()
	if err != nil {
		return err
	}
	secret.Data = map[string][]byte{
		v1.DockerConfigJsonKey: data,
	}
	if _, err := service.client.CoreV1().Secrets(nameSpace).Create(secret); err != nil {
		return err
	}
	return nil
}

// dockerConfigJson returns a json rep of user's
// docker registry auth credentials.
func (service *PaasK8sService) dockerConfigJson() ([]byte, error) {
	// {"auths": {"yourprivateregistry.com":
	// {"username":"janedoe",
	// "password":"xxxxxxxxxxx",
	// "email":"jdoe@example.com",
	// "auth":"c3R...zE2"}}}
	type authData struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Auth     string `json:"auth"`
	}
	username, password := service.registry.Username, service.registry.Password
	ad := authData{
		Username: username,
		Password: password,
	}
	type auths struct {
		Auths map[string]authData `json:"auths"`
	}
	usernamePassword := fmt.Sprintf("%s:%s", username, password)
	encoded := base64.StdEncoding.EncodeToString([]byte(usernamePassword))
	ad.Auth = encoded
	a := &auths{Auths: map[string]authData{
		service.registry.RegistryUrl: ad,
	}}
	return json.Marshal(a)
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
		ImagePullSecrets: []v1.LocalObjectReference{{Name: secretName}},
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
	// add re-write annotations
	// so that app can be accessed from the same host
	// kubernetes.io/ingress.class: "nginx"
	// nginx.ingress.kubernetes.io/rewrite-target: /$1
	ingress.Annotations = map[string]string{
		"kubernetes.io/ingress.class":                "nginx",
		"nginx.ingress.kubernetes.io/rewrite-target": "/$1",
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
	labels := map[string]string{"database": name}
	svc.Name = name
	svc.Labels = labels
	svc.Spec = v1.ServiceSpec{
		Selector: labels,
		Type:     v1.ServiceTypeLoadBalancer,
		Ports:    []v1.ServicePort{{Name: "db-port", Protocol: "TCP", Port: port}},
	}
	if _, err := service.client.CoreV1().Services(nameSpace).Create(svc); err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) createPersistentVolume(name string, size int64) error {
	labels := map[string]string{"type" : fmt.Sprintf("%s-local", name)}
	pv := &v1.PersistentVolume{}
	pv.Name = fmt.Sprintf("%s-pv", name)
	pv.Labels = labels
	spec := v1.PersistentVolumeSpec{
		Capacity: map[v1.ResourceName]resource.Quantity{
			v1.ResourceStorage: *resource.NewQuantity(size, resource.BinarySI),
		},
		AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
		StorageClassName: "manual",
	}
	spec.PersistentVolumeSource = v1.PersistentVolumeSource{
		HostPath: &v1.HostPathVolumeSource{Path: "/mnt/data"},
	}
	pv.Spec = spec
	if _, err := service.client.CoreV1().PersistentVolumes().Create(pv); err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) createPersistentVolumeClaim(name string, size int64) error {
	pvc := &v1.PersistentVolumeClaim{}
	pvc.Name = fmt.Sprintf("%s-pvc", name)
	spec := v1.PersistentVolumeClaimSpec{
		StorageClassName: String("manual"),
		AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
		Resources: v1.ResourceRequirements{
			Requests: map[v1.ResourceName]resource.Quantity{
				v1.ResourceStorage: *resource.NewQuantity(size, resource.BinarySI),
			},
		},
	}
	pvc.Spec = spec
	if _, err := service.client.CoreV1().PersistentVolumeClaims(nameSpace).Create(pvc); err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) createDatabaseDeployment(opt *types.ProvisionDatabaseOpts) error {
	secName, secValue := fmt.Sprintf("%s-secret", opt.Name), opt.Envs[opt.PasswordKey]
	if err := service.createSecret(secName, secValue); err != nil {
		return err
	}
	envs := make([]v1.EnvVar, 0, len(opt.Envs))
	envs = append(envs, v1.EnvVar{
		Name: opt.PasswordKey,
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{Key: secName},
		},
	})
	// prepare environment variables
	for k, v := range opt.Envs {
		envs = append(envs, v1.EnvVar{Name: k, Value: v})
	}

	deployment := &appsv1.Deployment{}
	name := fmt.Sprintf("%s-database-deployment", opt.Name)
	labels := map[string]string{"database": name}
	spec := appsv1.DeploymentSpec{}
	spec.Selector = &metav1.LabelSelector{
		MatchLabels: labels,
	}
	spec.Strategy = appsv1.DeploymentStrategy{
		Type: appsv1.RecreateDeploymentStrategyType,
	}
	volumeMountName := fmt.Sprintf("%s-volume-mount")
	template := v1.PodTemplateSpec{}
	template.Labels = labels
	container := v1.Container{
		Name: fmt.Sprintf("%s-container", name),
		Image: opt.BaseImage,
		Ports: []v1.ContainerPort{
			{Name: "connect-port", Protocol: "TCP", ContainerPort: opt.DefaultPort},
		},
		Env: envs,
		VolumeMounts: []v1.VolumeMount{{Name: volumeMountName, MountPath: opt.DataMountPath}},
	}
	template.Spec.Containers = []v1.Container{container}
	template.Spec.Volumes = []v1.Volume{{Name: volumeMountName, VolumeSource: v1.VolumeSource{
		PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: fmt.Sprintf("%s-pvc", opt.Name)},
	}}}
	spec.Template = template
	deployment.Name = name
	deployment.Labels = labels
	deployment.Spec = spec
	if _, err := service.client.AppsV1().Deployments(nameSpace).Create(deployment); err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) createSecret(name, value string) error {
	secret := &v1.Secret{}
	secret.Name = name
	secret.Type = v1.SecretTypeOpaque
	secret.StringData = map[string]string{
		name: base64.StdEncoding.EncodeToString([]byte(value)),
	}
	if _, err := service.client.CoreV1().Secrets(nameSpace).Create(secret); err != nil {
		return err
	}
	return nil
}

func (service *PaasK8sService) ProvisionDatabase(opt *types.ProvisionDatabaseOpts) (*types.DatabaseProvisionResult, error) {
	if err := service.createPersistentVolume(opt.Name, opt.Space); err != nil {
		return nil, err
	}
	if err := service.createPersistentVolumeClaim(opt.Name, opt.Space); err != nil {
		return nil, err
	}
	if err := service.createDatabaseLbService(opt.Name, opt.DefaultPort); err != nil {
		return nil, err
	}
	if err := service.createDatabaseDeployment(opt); err != nil {
		return nil, err
	}
	return &types.DatabaseProvisionResult{
		Credential: &types.DatabaseCredential{
			Username: opt.Envs[opt.UsernameKey],
			Password: opt.Envs[opt.PasswordKey],
			DatabaseName: opt.Envs[opt.DatabaseNameKey],
		},
	}, nil
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
	lbs := ingress.Status.LoadBalancer.Ingress[0]
	log.Println(lbs)
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
