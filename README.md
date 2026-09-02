# inference-operator
📝 项目详解：[从零构建一个 AI 推理服务 Operator](https://www.cnblogs.com/lblblog/articles/22055524)

一个基于 Kubernetes Operator 模式的 AI 推理服务管理平台，实现模型服务的声明式部署、自动扩缩、健康检查和生命周期管理。

## 功能特性

- **声明式管理**：通过自定义资源 InferenceService 定义推理服务，Operator 自动创建底层 Deployment 和 Service
- **动态扩缩容**：修改 CR 的 replicas 字段，Operator 自动调整 Deployment 副本数
- **健康检查**：实时监控推理服务就绪副本数，更新 CR 状态为 Running/Pending/Unhealthy
- **优雅清理**：通过 Finalizer 机制，删除 CR 时自动清理关联的 Deployment 和 Service
- **离线部署**：推理服务镜像已包含模型文件，无需下载即可启动
- **可观测性**：支持 Prometheus Metrics 暴露（规划中）

## 架构设计

``` mermaid
graph TD
    User[用户] -->|kubectl apply| CR[InferenceService CR]
    CR -->|触发| Reconcile[Controller Reconcile]
    Reconcile -->|创建/更新| Deploy[Deployment]
    Reconcile -->|创建/更新| Svc[Service]
    Deploy -->|管理| Pod[推理服务 Pod]
    Svc -->|暴露端口| Pod
    Reconcile -->|更新| Status[CR Status]
```

## API 规范

### Spec 字段
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| modelName | string | 否 | 模型标识，用于标签 |
| image | string | 是 | 推理服务 Docker 镜像 |
| replicas | int32 | 否 | 副本数，默认为 1 |

### Status 字段
| 字段 | 类型 | 说明 |
|------|------|------|
| readyReplicas | int32 | 已就绪的 Pod 数量 |
| phase | string | Running / Pending / Unhealthy |
| healthy | bool | 健康检查是否通过 |


## 快速开始

### 前置条件
- Kubernetes 集群（可使用 Kind 创建）
- Go 1.21+
- Docker

### 1. 安装 CRD
``` bash
git clone https://github.com/2388522369/inference-operator.git
cd inference-operator
make install
```

### 2. 启动 Controller
``` bash
make run
```

### 3. 部署推理服务
``` bash
kubectl apply -f config/samples/ai_v1_inferenceservice.yaml
```

### 4. 验证
```bash
kubectl get inferenceservices
kubectl get pods -l app=inference
kubectl port-forward service/sentiment-service 8000:8000
curl http://localhost:8000/health
```

## 技术栈
- **语言与框架**：Go, Kubebuilder, controller-runtime
- **Kubernetes 资源**：CRD, Deployment, Service, Finalizer
- **推理服务**：Python, FastAPI, HuggingFace Transformers
- **容器化**：Docker

## 项目亮点
1. 完整实现了 Operator 的 CRUD 闭环，覆盖创建、更新、健康检查、Finalizer 清理
2. 解决集群外 Operator 无法访问 Service 的本地开发问题，提出基于就绪副本数的轻量方案
3. 推理服务镜像离线可用，模型文件直接打包，无需运行时下载
4. 代码注释完整，具备生产级项目的错误处理和日志规范

## 补充
### 1.kind-config.yaml
由于网络原因容器无法从Docker Hub上拉取镜像，因此我把推理服务镜像打包发送到了本地仓库，但是创建的Kind 节点的 containerd 默认会拒绝从 HTTP 仓库拉取，因此在创建集群时要带上信任本地仓库的配置文件kind-config.yaml。
``` yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."172.17.0.1:5000"]
    endpoint = ["http://172.17.0.1:5000"]
```
``` bash
# 用新配置重建
kind create cluster --name ai-infra --config kind-config.yaml
```
### 2.port-forward
由于Opeartor未运行在集群内部，所以在访问集群内部服务时需转发端口实现访问。在本地开一个隧道，把 localhost:8080 的流量通过 API Server 转发到集群里的 Pod 或 Service。它绕过了 ClusterIP 不可达的问题，因为流量走的是 kubectl → API Server → Pod

获取pod name：
``` bash
kubectl get pods -l app=inference
```
转发端口：
``` bash
kubectl port-forward service/<pod name> 8080:8000
```
然后访问：
``` bash
curl -X POST http://localhost:8080/predict \
  -H "Content-Type: application/json" \
  -d '{"text": "I love this"}'
```