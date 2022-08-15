# KS API Server 验证模块的实现 (待完成)

作为在[这篇分析](./api-server-webservice-handler.md)中提到的请求 handler 链中的第一个 filter，Authentication 模块对请求进行了验证，并拦截不合法的请求。

将 Authentication 加入到请求处理链中的代码如下：

```go
userLister := s.InformerFactory.KubeSphereSharedInformerFactory().Iam().V1alpha2().Users().Lister()
loginRecorder := auth.NewLoginRecorder(s.KubernetesClient.KubeSphere(), userLister)

authn := unionauth.New(anonymous.NewAuthenticator(),
	basictoken.New(basic.NewBasicAuthenticator(auth.NewPasswordAuthenticator(
		s.KubernetesClient.KubeSphere(),
		userLister,
		s.Config.AuthenticationOptions),
		loginRecorder)),
	bearertoken.New(jwt.NewTokenAuthenticator(
		auth.NewTokenOperator(s.CacheClient, s.Issuer, s.Config.AuthenticationOptions),
		userLister)))
handler = filters.WithAuthentication(handler, authn)
```

从代码中得到的信息有：

- filter.WithAuthentication() 的第二个参数接收一个 [authenticator.Request](../vendor/k8s.io/apiserver/pkg/authentication/authenticator/interfaces.go) 的接口类型： 

    ```go
    // Request attempts to extract authentication information from a request and
    // returns a Response or an error if the request could not be checked.
    type Request interface {
    	AuthenticateRequest(req *http.Request) (*Response, bool, error)
    ```

- [unionauth.New()](../vendor/k8s.io/apiserver/pkg/authentication/request/union/union.go) 接收了三个 authenticator.Request 类型的参数，并返回 authn 并传入什么的 filter

- 用户 User 被设计成为一个 CRD，其在 baisictoken 和 bearertoken 中被读取使用

## filters.WithAuthentication()

```go
func WithAuthentication(handler http.Handler, authRequest authenticator.Request) http.Handler {
	if authRequest == nil {
		klog.Warningf("Authentication is disabled")
		return handler
	}
	s := serializer.NewCodecFactory(runtime.NewScheme()).WithoutConversion()

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		resp, ok, err := authRequest.AuthenticateRequest(req)
		_, _, usingBasicAuth := req.BasicAuth()

		defer func() {
			// if we authenticated successfully, go ahead and remove the bearer token so that no one
			// is ever tempted to use it inside of the API server
			if usingBasicAuth && ok {
				req.Header.Del("Authorization")
			}
		}()

		if err != nil || !ok {
			ctx := req.Context()
			requestInfo, found := request.RequestInfoFrom(ctx)
			if !found {
				responsewriters.InternalError(w, req, errors.New("no RequestInfo found in the context"))
				return
			}
			gv := schema.GroupVersion{Group: requestInfo.APIGroup, Version: requestInfo.APIVersion}
			responsewriters.ErrorNegotiated(apierrors.NewUnauthorized(fmt.Sprintf("Unauthorized: %s", err)), s, gv, w, req)
			return
		}

		req = req.WithContext(request.WithUser(req.Context(), resp.User))
		handler.ServeHTTP(w, req)
	})
}
```

从上面的代码中有以下信息：

- 如果验证功能关闭，则不做任何事情，直接执行链中下一个 handler

- 调用 authRequest 接口的 AuthenticateRequest() 方法验证 req

- 如果未通过验证，则将错误写回客户端

- 如果验证通过，将验证过程中提取的 User 写进 req 的上下文中，执行链中的下一个 handler

- 如果是使用的 basicAuth (用户名+密码)，则在验证通过后，从请求 Header 中将验证信息移除

## unionAuth 以及其 AuthenticateRequest 方法的实现

最开始的 authn 是三个验证方法的集合：匿名验证，basic用户名+密码验证，以及 bearerToken 验证。在[其实现](../vendor/k8s.io/apiserver/pkg/authentication/request/union/union.go)中，三个验证方法被放入一个切片中，并在最终实现的 AuthenticateRequest 方法中有以下的行为：

- 遍历所有验证方法 AuthenticateRequest，如果有任何一个成功，则验证通过

- 当所有验证方法都失败时，返回验证失败，和验证的错误消息

所以鉴权模块的学习最终落到了 [pkg/apiserver/authentication/request包](../pkg/apiserver/authentication/request) 中三个子包中类型实现的 AuthenticateRequest 方法的实现上：

- [anonymous](../pkg/apiserver/authentication/request/anonymous)

- [basictoken](../pkg/apiserver/authentication/request/basictoken)

- [bearertoken](../pkg/apiserver/authentication/request/bearertoken)

(具体鉴权模块的分析待续)
