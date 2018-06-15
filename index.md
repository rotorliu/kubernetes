# Repository configuration

In order to setup the kubernetes repository for your distribution, follow the instructions below.

## Ubuntu distributions

#### Xenial x86_64

```bash
curl -s -L https://nvidia.github.io/kubernetes/gpgkey | \
  sudo apt-key add -
curl -s -L https://nvidia.github.io/kubernetes/ubuntu16.04/nvidia-kubernetes.list | \
  sudo tee /etc/apt/sources.list.d/nvidia-kubernetes.list
sudo apt-get update
```
