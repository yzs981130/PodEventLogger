# Pod Event Logger

## documentation

通过轮询k8s中的event endpoint`api/v1/namespaces/default/events`，只将发生了更新的event记录下来，并持久化写入log中。

## usage

`--kubeconfig` 指定kubeconfig，无则用`InCluster()`config

`--logdir` 指定log的目录，必须具有写入权限，默认为`/log`，建议将host目录挂volume到`/log`下

## image

`docker pull yzs981130/podeventlogger:version-0.0.4`

