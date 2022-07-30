# kubesphere api-server 的启动流程

## 一，入口：*cobra.Command 与 NewAPIServerCommand()

在 api-server 的 [main()](../cmd/ks-apiserver/apiserver.go) 函数中，调用 app.NewAPIServerCommand() 创建了 *cobra.Command 实例执行其 Execute() 方法启动 api-server：

```go
func main() {
	cmd := app.NewAPIServerCommand()
	cmd.Execute()
}
```

[Cobra](https://github.com/spf13/cobra)是一个用于创建现代的 cli 应用程序的库。同时它也是一个脚手架程序，用于生成命令行应用的框架。可以参考[《Go 每日一库之 cobra》](https://darjun.github.io/2020/01/17/godailylib/cobra/) 学习它的大致用法。

在 [NewAPIServerCommand()](../cmd/ks-apiserver/app/server.go) 中，首先从文件读取了配置，并用于 ServerRunOptions 的初始化：

```go
s := options.NewServerRunOptions()

// Load configuration from file
conf, err := apiserverconfig.TryLoadFromDisk()
if err == nil {
	s = &options.ServerRunOptions{
		GenericServerRunOptions: s.GenericServerRunOptions,
		Config:                  conf,
	}
}
```

上方的代码中，第一次被赋值的变量 s 只在后面的第二次赋值中提供 GenericServerRunOptions 字段的值。所以可以写成：

下面的代码已经在 [PR#5108](https://github.com/kubesphere/kubesphere/pull/5108) 里合入。

```go
s := options.NewServerRunOptions()

conf, err := apiserverconfig.TryLoadFromDisk()
if err == nil {
	s.Config = conf
} else {
	klog.Fatal("Failed to load configuration from disk", err)
}
```

接着声明并初始化了变量 cmd，并在为其 RunE 字段赋值的匿名函数中，校验了参数然后调用 Run() 来启动 api-server。RunE 字段的值，最终会在 cmd.Execure() 执行的时候被调用。

api-server 的配置系统设计与读取，在[这篇分析](./api-server-config.md)中介绍。

```go
cmd := &cobra.Command{
	//...
	RunE: func(cmd *cobra.Command, args []string) error {
		if errs := s.Validate(); len(errs) != 0 {
			return utilerrors.NewAggregate(errs)
		}
		//...
		return Run(s, apiserverconfig.WatchConfigChange(), signals.SetupSignalHandler())
	},
	//...
}
```

最后将 s 中的 flagset 逐个放入 cmd 中，帮助 cmd 的 HelpFunc 帮助函数工作。而 apr-server 的参数还是靠 s 变量来传递：

```go
fs := cmd.Flags()
namedFlagSets := s.Flags()
for _, f := range namedFlagSets.FlagSets {
	fs.AddFlagSet(f)
}
```

最终 cmd 被返回给调用者 main() 函数。

### 检测配置的变动并重启

在 cmd.RunE 被赋值的 Run() 函数中，一开始从 context 创建了退出通知函数 cancelFunc，并启动了一个 goroutine 等待真正运行 server 的 run 函数的退出：

```go
ictx, cancelFunc := context.WithCancel(context.TODO())
errCh := make(chan error)
defer close(errCh)
go func() {
	if err := run(s, ictx); err != nil {
		errCh <- err
	}
}()
```

在接下来的无限循环中，监听了下面三个通道:

- [signals.SetupSignalHandler()](../vendor/sigs.k8s.io/controller-runtime/pkg/manager/signals/signal.go) 返回的 ctx.Done()

  当进程收到了 SIGINT 或者 SIGTERM 信号时，只通知 api server 停止。

- errCh 通道

  当 api server 发生错误时，调用 cancel 函数停止，并返回错误

- 监听 config 文件变化的通道

  读取配置文件的 viper 库支持监听配置文件的修改，并通过通道发送通知。此时更新 s 中的 Config 以及上下文 ictx，重新在 goroutine 中调用 run 启动新的 api server

## 二，run()：初始化 APIServer

在上面的 goroutine 中执行的 run() 函数中，首先从配置项集合 s 创建出 ApiServer 的实例：

```go
apiserver, err := s.NewAPIServer(ctx.Done())
```

从 APIServer 结构体的[定义](../pkg/apiserver/apiserver.go) 中，可以看出其包含了以下几类字段：

- 一个 *http.Server 以及包含 web server 路由信息的 *restful.Container

- k8s client-go 的 Client 客户端， 各种 k8s 原生以及非原生资源的 informer 工厂，以及 runtime 缓存

- kubesphere 平台中各个模块的客户端接口

所以在最终调用 http 服务器的监听方法前，所有的工作都集中在初始化这些字段上。

在 NewAPIServer() 的开头，用配置文件中的 .kube/config 文件，初始化了一系列 k8s 客户端的集合，以及它们的 informer 工厂的集合：

```go
kubernetesClient, err := k8s.NewKubernetesClient(s.KubernetesOptions)
apiServer.KubernetesClient = kubernetesClient

informerFactory := informers.NewInformerFactories(kubernetesClient.Kubernetes(), kubernetesClient.KubeSphere(),
	kubernetesClient.Istio(), kubernetesClient.Snapshot(), kubernetesClient.ApiExtensions(), kubernetesClient.Prometheus())
apiServer.InformerFactory = informerFactory
```

这些 k8s 客户端或者 informer 有：

- 原生 k8s 资源客户端：k8s

- kubesphere 自定义资源的客户端：ks

- istio 资源客户端

- k8s-csi snapshot 的客户端

- k8s apiextensions 客户端

- prometheus 客户端

随后就开始了一系列平台模块客户端的初始化，以日志模块为例：

```go
if s.MonitoringOptions == nil || len(s.MonitoringOptions.Endpoint) == 0 {
	return nil, fmt.Errorf("moinitoring service address in configuration MUST not be empty, please check configmap/kubesphere-config in kubesphere-system namespace")
} else {
	monitoringClient, err := prometheus.NewPrometheus(s.MonitoringOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to prometheus, please check prometheus status, error: %v", err)
	}
	apiServer.MonitoringClient = monitoringClient
}
```

当 ks 配置文件中 monitoring 一节有配置信息时，就用它初始化一个 prometheus 客户端，并赋值给 apiServer 相关字段。其余客户端的初始化与赋值步骤差不多，在此只给出它们的列表：

- 监控模块 prometheus 客户端

- 指标 MetricsClient

- 日志 elasticSearch 客户端

- S3 客户端

- DevOps Jenkins 客户端

- 镜像扫描 SonarQube 客户端

- 缓存模块 Redis 客户端

- 基于 es 的事件模块 events

- 基于 es 的审计模块 auditing

- 告警模块 alerting

- 多集群管理模块 cluster 客户端

- 基于 Openpitrix 的应用市场客户端

- 认证模块 issuer

在 kubesphere 中，有很多自定义 k8s 资源 CRD。 每一种 CRD 都有各自的 group version kind (gvk) 三元组来标识它。在接下来的代码将 CRD 的 API 加入到 scheme.Scheme 中，目的是为了将 gvk 与定义 CRD 的结构体对应起来：

```go
sch := scheme.Scheme
if err := apis.AddToScheme(sch); err != nil {
	klog.Fatalf("unable add APIs to scheme: %v", err)
}
```

在 [pkg/apis](https://github.com/kubesphere/kubesphere/tree/master/pkg/apis) 包中，每个文件的 init() 函数将各自的 gvk 加入到 scheme 中。从这个包中，也可以看到 ks 自定义的 crd 有哪些。


## 三，PrepareRun() 和 Run(): 配置 HttpHandler 与运行 Http.Server

在 apiserver 的 PrepareRun() 方法中，配置了 kubesphere 的 *http.Server 的 Handler。由于流程比较复杂，另起了一个[文档](./api-server-webservice-handler.md)来记录。

在 Run() 方法中，首先从集群同步了 informer 中的资源，最后调用了 ListenAndServer() 启动了 http 服务