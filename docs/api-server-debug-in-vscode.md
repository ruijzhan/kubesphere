# 在 VS Code 中 debug API Server

## launch.json

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "ks-apiserver",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${workspaceFolder}/cmd/ks-apiserver/apiserver.go",
            "args": ["--gops"]
        }
    ]
}
```

## 配置文件 kubesphere.yaml

将命令 "kubectl -n kubesphere-system get cm kubesphere-config -o yaml" 的输出保存到文件 cmd/ks-apiserver/kubesphere.yaml 中，并在其中加入 k8s api server 的连接配置：

```yaml
kubernetes:
  kubeconfig: "/home/user/.kube/config"
  master: https://192.168.12.250:6443
  $qps: 1e+06
  burst: 1000000
```

然后在 VS Code 中按 F5 就可以 debug 了。