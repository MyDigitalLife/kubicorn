#!/usr/bin/env bash
set -e
cd ~

# ------------------------------------------------------------------------------------------------------------------------
# These values are injected into the script. We are explicitly not using a templating language to inject the values
# as to encourage the user to limit their use of templating logic in these files. By design all injected values should
# be able to be set at runtime, and the shell script real work. If you need conditional logic, write it in bash
# or make another shell script.
#
#
TOKEN="INJECTEDTOKEN"
PORT="INJECTEDPORT"
KUBEADM_VERSION="INJECTEDKUBEADMVERSION"
K8S_VERSION="INJECTEDK8SVERSION"
#
#
# ------------------------------------------------------------------------------------------------------------------------

# Disabling SELinux is not recommended and will be fixed later.
sudo sed -i 's/^SELINUX=.*/SELINUX=disabled/g' /etc/sysconfig/selinux
sudo setenforce 0

sudo rpm --import https://packages.cloud.google.com/yum/doc/yum-key.gpg
sudo rpm --import https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg

sudo sh -c 'cat <<EOF > /etc/yum.repos.d/kubernetes.repo
[kubernetes]
name=Kubernetes
baseurl=http://yum.kubernetes.io/repos/kubernetes-el7-x86_64
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg
       https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
EOF'

sudo yum makecache -y
sudo sudo yum install -y \
     docker \
     socat \
     ebtables \
     kubelet \
     kubeadm=${KUBEADM_VERSION} \
     cloud-utils

sudo systemctl enable docker
sudo systemctl enable kubelet.service
sudo systemctl start docker

PUBLICIP=$(ec2metadata --public-ipv4 | cut -d " " -f 2)
PRIVATEIP=$(ip addr show dev eth0 | awk '/inet / {print $2}' | cut -d"/" -f1)

# Required by kubeadm
sudo sysctl -w net.bridge.bridge-nf-call-iptables=1
sudo sysctl -p

kubeadm reset
kubeadm init --apiserver-bind-port ${PORT} --token ${TOKEN} --kubernetes-version ${K8S_VERSION} --apiserver-advertise-address ${PUBLICIP} --apiserver-cert-extra-sans ${PUBLICIP} ${PRIVATEIP}

# Thanks Kelsey :)
kubectl apply \
  -f http://docs.projectcalico.org/v2.3/getting-started/kubernetes/installation/hosted/kubeadm/1.6/calico.yaml \
  --kubeconfig /etc/kubernetes/admin.conf

# Default centos user
mkdir -p /home/centos/.kube
cp /etc/kubernetes/admin.conf /home/centos/.kube/config
chown -R centos:centos /home/centos/.kube

