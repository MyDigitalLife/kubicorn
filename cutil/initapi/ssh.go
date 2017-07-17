package initapi

import (
	"fmt"
	"github.com/gravitational/teleport/lib/sshutils"
	"github.com/kris-nova/klone/pkg/local"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"io/ioutil"
	"strings"
)

func sshLoader(initCluster *cluster.Cluster) (*cluster.Cluster, error) {
	if initCluster.Ssh.PublicKeyPath != "" {
		fmt.Println(initCluster.Ssh.PublicKeyPath)
		bytes, err := ioutil.ReadFile(local.Expand(initCluster.Ssh.PublicKeyPath))
		if err != nil {
			return nil, err
		}
		initCluster.Ssh.PublicKeyData = bytes
		privateBytes, err := ioutil.ReadFile(strings.Replace(local.Expand(initCluster.Ssh.PublicKeyPath), ".pub", "", 1))
		if err != nil {
			return nil, err
		}
		fp, err := sshutils.PrivateKeyFingerprint(privateBytes)
		if err != nil {
			return nil, err
		}
		initCluster.Ssh.PublicKeyFingerprint = fp
	}

	return initCluster, nil
}
