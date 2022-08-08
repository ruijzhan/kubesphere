# kubesphere API 的注册与 handler 绑定 (未完成)

API 的路由以及 handler 的配置主要在 PrepareRun() 方法中完成:

```go
func (s *APIServer) PrepareRun(stopCh <-chan struct{}) error {
	s.container = restful.NewContainer()
	s.container.Filter(logRequestAndResponse)
	s.container.Router(restful.CurlyRouter{})
	
	// 恢复 panic 并记录日志
	s.container.RecoverHandler(func(panicReason interface{}, httpWriter http.ResponseWriter) {
		logStackOnRecover(panicReason, httpWriter)
	})

	// 为 kubesphere, crd 与 metrics 的 API 注册路由
	s.installKubeSphereAPIs(stopCh)
	s.installCRDAPIs()
	s.installMetricsAPI()

	// 统计请求的次数以及每个 API 请求的耗时
	s.container.Filter(monitorRequest)

	for _, ws := range s.container.RegisteredWebServices() {
		klog.V(2).Infof("%s", ws.RootPath())
	}

	s.Server.Handler = s.container
	
	// 配置其他前置 handler 链，再另一篇文档中有分析
	s.buildHandlerChain(stopCh)

	return nil
}
```

kubesphere 的 API server 用了 [go-restful](https://github.com/emicklei/go-restful) 来构建 restful 风格的 web service。其主要使用了 go-restful 包中以下几个类型：

- **Container**: 多个 Webservice 的集合，并实现了 ServeHTTP 方法。kubesphere 所有自己实现的 api 都放在 s.container 中

- **Webservice**: 在 ks 的 api 路径形式为：/kapis/\<GROUP\>/\<VERSION\>/...。一个 Webservice 对应一个 group+version，提供了初步的路由分类。一个 Webservice 是多个 Route 的集合。

- **Route**: 具体到整个 API 路径 + http 方法与 handler 的绑定。


## s.installKubeSphereAPIs(stopCh):

ks 在 [pkg/kapis包](../pkg/kapis/) 下实现了平台各个模块的 API。每个 API 对应的包中，都实现了 AddToContainer() 函数将自己所有 API 的 Path 以及对应的 Handler 添加到 server 中总的 container 中。最终的学习路径就落到了对于每个 API 所调用的后端模块客户端，以及不同路径的 API 对应的 Handler 的学习上。

在此大致列出 ks 的 API 以及其总路径，注意有的 API 有多个版本。具体代码见[pkg/apiserver/apiserver.go](../pkg/apiserver/apiserver.go)

- 集群配置 [config/v1alpha2/](../pkg/kapis/config/v1alpha2/register.go)

- 资源管理 [resources/v1alpha3/](../pkg/kapis/resources/v1alpha3/register.go)

- 监控 [monitoring/v1alpha3/](../pkg/kapis/monitoring/v1alpha3/register.go)

- 指标 [metering/v1alpha1/](../pkg/kapis/metering/v1alpha1/register.go)

- 应用市场 [openpitrix/v1/](../pkg/kapis/openpitrix/v1/register.go)

- Job 管理 [operations/v1alpha2/](../pkg/kapis/operations/v1alpha2/register.go)

- workspace 管理 [tenant/v1alpha3/](../pkg/kapis/tenant/v1alpha3/register.go)

- Pod 命令行终端 [terminal/v1alpha2/](../pkg/kapis/terminal/v1alpha2/register.go)

- 多集群管理 [cluster/v1alpha1/](../pkg/kapis/cluster/v1alpha1/register.go)

- iam 用户管理 [iam/v1alpha2/](../pkg/kapis/iam/v1alpha2/register.go)

- 验证鉴权 [oauth/](../pkg/kapis/oauth/register.go)

- 服务网格 [servicemesh/metrics/v1alpha2/](../pkg/kapis/servicemesh/metrics/v1alpha2/register.go)

- 容器网络 [network/v1alpha2/](../pkg/kapis/network/v1alpha2/register.go)

- DevOps [devops/](../pkg/kapis/devops/register.go)

- 通知 [notification/v1/](../pkg/kapis/notification/v1/register.go)

- 告警 [alerting/v2beta1/](../pkg/kapis/alerting/v2beta1/register.go)

- 边缘计算 [kubeedge/v1alpha1/](../pkg/kapis/kubeedge/v1alpha1/register.go)

- 网关 [gateway/v1alpha1/](../pkg/kapis/gateway/v1alpha1/register.go)
