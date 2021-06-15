# Pelion

Instructions on building and running kube-router in Pelion

## Requirements

Building requirements:
- make: https://man7.org/linux/man-pages/man1/make.1.html
- Docker (optional): https://docs.docker.com/engine/install/ubuntu/

Runtime requirement - ipset
```
apt update && apt install -y ipset
```

Referenced for additional information (not required in Pelion), [kube-router official requirements](https://www.kube-router.io/docs/user-guide/#requirements)

## Building

**With docker**

Build a binary optimized for the machine running the command (e.g. amd64)
```
make kube-router
```

Build for some other target architecture: `arm`, `arm64`, `s390x`, `ppc64le`, `amd64`
```
make container GOARCH=arm64
```

**Without docker**

Requires golang v1.13 or greater

```
BUILD_IN_DOCKER=false make kube-router
```

## Running
Start kube-router without overlay, ibgp and proxy.
```
kube-router --kubeconfig=/path/to/kubeconfig \
--run-firewall=true \
--run-service-proxy=false \
--run-router=true \
--master=127.0.0.1:10355 \
--enable-overlay=false \
--enable-ibgp=false \
--hostname-override=<Device ID> \
--pod-cidr=<PodCIDR>
```

The kubeconfig must conform to the standard [Kubernetes kubeconfig](https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/) but may be empty. The `--master` flag will override the server location from the config. The endpoint provided in this flag must connect to the apiserver.

- `Device ID`: Gateway ID
- `PodCIDR`: IP range to use for Pods on this Node, e.g. `10.240.0.0/24`
