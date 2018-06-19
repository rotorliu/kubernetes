# Kubernetes on NVIDIA GPUs

[![Submit Queue Widget]][Submit Queue] [![GoDoc Widget]][GoDoc] [![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/569/badge)](https://bestpractices.coreinfrastructure.org/projects/569)

<img src="https://github.com/kubernetes/kubernetes/raw/master/logo/logo.png" width="100">

----

Kubernetes is an open source system for managing [containerized applications]
across multiple hosts, providing basic mechanisms for deployment, maintenance,
and scaling of applications.

Kubernetes builds upon a decade and a half of experience at Google running
production workloads at scale using a system called [Borg],
combined with best-of-breed ideas and practices from the community.

[Kubernetes on NVIDIA GPUs] includes support for GPUs and enhancements to 
Kubernetes, so users can easily configure and use GPU resources for accelerating 
deep learning workloads.

----

## To start using Kubernetes

Get started with Kubernetes on NVIDIA GPUs by reviewing the [installation guide].
 
The general Kubernetes documentation is available at [kubernetes.io].

## Support

General [troubleshooting guidelines] are available in the documentation. Feel free 
to also open an issue on GitHub or post questions on the NVIDIA [Developer Forums]. 

For general Kubernetes issues, start with the [troubleshooting guide].

## Release Highlights

### Supported Platforms

This release of Kubernetes is supported on the following platforms.

#### On-Prem

* [DGX-1] with OS [Server v3.1.6]
* [DGX-Station] with OS [Desktop v3.1.6]

#### Cloud

[NVIDIA GPU Cloud] virtual machine images available on Amazon EC2 and 
Google Cloud Platform.

### New Features

* Support for NVIDIA GPUs in Kubernetes using the NVIDIA device plugin
* Support for GPU attributes such as GPU type and memory requirements via the 
Kubernetes PodSpec
* Visualize and monitor GPU metrics and health with an integrated GPU monitoring stack of [NVIDIA DCGM], Prometheus and Grafana
* Support for Docker and CRI-O using the NVIDIA Container Runtime


[announcement]: https://cncf.io/news/announcement/2015/07/new-cloud-native-computing-foundation-drive-alignment-among-container
[Borg]: https://research.google.com/pubs/pub43438.html
[CNCF]: https://www.cncf.io/about
[communication]: https://github.com/kubernetes/community/blob/master/communication.md
[community repository]: https://github.com/kubernetes/community
[containerized applications]: https://kubernetes.io/docs/concepts/overview/what-is-kubernetes/
[developer's documentation]: https://github.com/kubernetes/community/tree/master/contributors/devel
[Docker environment]: https://docs.docker.com/engine
[Go environment]: https://golang.org/doc/install
[GoDoc]: https://godoc.org/k8s.io/kubernetes
[GoDoc Widget]: https://godoc.org/k8s.io/kubernetes?status.svg
[interactive tutorial]: http://kubernetes.io/docs/tutorials/kubernetes-basics
[kubernetes.io]: http://kubernetes.io
[Scalable Microservices with Kubernetes]: https://www.udacity.com/course/scalable-microservices-with-kubernetes--ud615
[Submit Queue]: http://submit-queue.k8s.io/#/ci
[Submit Queue Widget]: http://submit-queue.k8s.io/health.svg?v=1
[troubleshooting guide]: https://kubernetes.io/docs/tasks/debug-application-cluster/troubleshooting/ 
[Kubernetes on NVIDIA GPUs]: https://developer.nvidia.com/kubernetes-gpu
[installation guide]: http://docs.nvidia.com/datacenter/index.html#kubernetes
[troubleshooting guidelines]: http://docs.nvidia.com/datacenter/kubernetes-install-guide/index.html#kubernetes_troubleshooting
[Developer Forums]: https://devtalk.nvidia.com/default/board/317/kubernetes-on-nvidia-gpus-/
[NVIDIA GPU Cloud]: https://docs.nvidia.com/ngc/index.html
[DGX-1]: https://docs.nvidia.com/dgx/dgx1-user-guide/index.html
[DGX-Station]: https://docs.nvidia.com/dgx/dgx-station-user-guide/index.html
[Server v3.1.6]: https://docs.nvidia.com/dgx/dgx-os-server-release-notes/index.html
[Desktop v3.1.6]: https://docs.nvidia.com/dgx/dgx-os-desktop-release-notes/index.html
[NVIDIA DCGM]: https://developer.nvidia.com/data-center-gpu-manager-dcgm


[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/README.md?pixel)]()
