# 在 KinD 集群中部署 kubesphere

[KinD](https://kind.sigs.k8s.io/) 是一种在本地机器 Docker 中部署的 k8s 集群，可以非常方便的创建和销毁。利用 KinD 可以快速的部署一套 kubesphere 用于开发调试。

## 操作步骤：

1，使用下方的集群配置文件，创建出一个 k8s 集群：

```yaml
# kind.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:v1.22.15@sha256:bfd5eaae36849bfb3c1e3b9442f3da17d730718248939d9d547e86bbac5da586
  extraPortMappings:
  - containerPort: 30880
    hostPort: 30880
  extraMounts:
  - hostPath: /etc/localtime
    containerPath: /etc/localtime
    readOnly: true
- role: worker
  image: kindest/node:v1.22.15@sha256:bfd5eaae36849bfb3c1e3b9442f3da17d730718248939d9d547e86bbac5da586
  extraMounts:
  - hostPath: /etc/localtime
    containerPath: /etc/localtime
    readOnly: true
- role: worker
  image: kindest/node:v1.22.15@sha256:bfd5eaae36849bfb3c1e3b9442f3da17d730718248939d9d547e86bbac5da586
  extraMounts:
  - hostPath: /etc/localtime
    containerPath: /etc/localtime
    readOnly: true
- role: worker
  image: kindest/node:v1.22.15@sha256:bfd5eaae36849bfb3c1e3b9442f3da17d730718248939d9d547e86bbac5da586
  extraMounts:
  - hostPath: /etc/localtime
    containerPath: /etc/localtime
    readOnly: true
```

执行命令等待集群创建完成

```sh
$ kind create cluster --config ./kind.yaml

Creating cluster "kind" ...
 ✓ Ensuring node image (kindest/node:v1.22.15) 🖼
 ✓ Preparing nodes 📦 📦 📦 📦  
 ✓ Writing configuration 📜 
 ✓ Starting control-plane 🕹️ 
 ✓ Installing CNI 🔌 
 ✓ Installing StorageClass 💾 
 ✓ Joining worker nodes 🚜 
Set kubectl context to "kind-kind"
You can now use your cluster with:

kubectl cluster-info --context kind-kind

Have a question, bug, or feature request? Let us know! https://kind.sigs.k8s.io/#community 🙂   
```

2，执行下方脚本在集群中部署 kubesphere，等待部署完成：

```sh
#!/bin/bash

for n in $(kubectl get node -o custom-columns=":metadata.name")
do
    echo $n
    docker exec -t ${n} bash -c "echo 'fs.inotify.max_user_watches=1048576' >> /etc/sysctl.conf"
    docker exec -t ${n} bash -c "echo 'fs.inotify.max_user_instances=512' >> /etc/sysctl.conf"
    docker exec -i ${n} bash -c "sysctl -p /etc/sysctl.conf"
done

kubectl apply -f https://github.com/kubesphere/ks-installer/releases/download/v3.3.0/kubesphere-installer.yaml
kubectl apply -f https://github.com/kubesphere/ks-installer/releases/download/v3.3.0/cluster-configuration.yaml
```

脚本中修改 inotify 相关参数源自[这个issue](https://kind.sigs.k8s.io/docs/user/known-issues/#pod-errors-due-to-too-many-open-files)，而真正有效的解决方法是参考的[这里](https://github.com/kubernetes-sigs/kind/issues/2586#issuecomment-1013614308)。

## 问题

Kubesphere 部署好过后有一些小问题，参考[此文档](./problems.md)进行解决。