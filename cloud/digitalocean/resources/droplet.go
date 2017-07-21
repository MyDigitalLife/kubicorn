package resources

import (
	"context"
	"fmt"
	"github.com/digitalocean/godo"
	"github.com/kris-nova/kubicorn/apis/cluster"
	"github.com/kris-nova/kubicorn/bootstrap"
	"github.com/kris-nova/kubicorn/cloud"
	"github.com/kris-nova/kubicorn/cutil/compare"
	"github.com/kris-nova/kubicorn/logger"
	"strconv"
	"time"
)

type Droplet struct {
	Shared
	Region         string
	Size           string
	Image          string
	Count          int
	SShFingerprint string
	ServerPool     *cluster.ServerPool
}

const (
	MasterIpAttempts               = 40
	MasterIpSleepSecondsPerAttempt = 3
)

func (r *Droplet) Actual(known *cluster.Cluster) (cloud.Resource, error) {
	logger.Debug("droplet.Actual")
	if r.CachedActual != nil {
		logger.Debug("Using cached droplet [actual]")
		return r.CachedActual, nil
	}
	actual := &Droplet{
		Shared: Shared{
			Name:    r.Name,
			CloudID: r.ServerPool.Identifier,
		},
	}

	if r.CloudID != "" {

		droplets, _, err := Sdk.Client.Droplets.ListByTag(context.TODO(), r.Name, &godo.ListOptions{})
		if err != nil {
			return nil, err
		}
		ld := len(droplets)
		if ld != 1 {
			return nil, fmt.Errorf("Found [%d] Droplets for Name [%s]", ld, r.Name)
		}
		droplet := droplets[0]
		id := strconv.Itoa(droplet.ID)
		actual.Name = droplet.Name
		actual.CloudID = id
		actual.Size = droplet.Size.Slug
		actual.Region = droplet.Region.Name
		actual.Image = droplet.Image.Slug
	}
	actual.SShFingerprint = known.Ssh.PublicKeyFingerprint
	actual.Count = r.ServerPool.MaxCount
	actual.Name = r.Name
	r.CachedActual = actual
	return actual, nil
}

func (r *Droplet) Expected(known *cluster.Cluster) (cloud.Resource, error) {
	logger.Debug("droplet.Expected")
	if r.CachedExpected != nil {
		logger.Debug("Using droplet subnet [expected]")
		return r.CachedExpected, nil
	}
	expected := &Droplet{
		Shared: Shared{
			Name:    r.Name,
			CloudID: r.ServerPool.Identifier,
		},
		Size:           r.ServerPool.Size,
		Region:         known.Location,
		Image:          r.ServerPool.Image,
		Count:          r.ServerPool.MaxCount,
		SShFingerprint: known.Ssh.PublicKeyFingerprint,
	}
	r.CachedExpected = expected
	return expected, nil
}

func (r *Droplet) Apply(actual, expected cloud.Resource, applyCluster *cluster.Cluster) (cloud.Resource, error) {
	logger.Debug("droplet.Apply")
	applyResource := expected.(*Droplet)
	isEqual, err := compare.IsEqual(actual.(*Droplet), expected.(*Droplet))
	if err != nil {
		return nil, err
	}
	if isEqual {
		return applyResource, nil
	}
	var userData []byte
	userData, err = bootstrap.Asset(fmt.Sprintf("bootstrap/%s", r.ServerPool.BootstrapScript))
	if err != nil {
		return nil, err
	}

	masterIpPrivate := ""
	masterIpPublic := ""
	if r.ServerPool.Type == cluster.ServerPoolType_Node {
		found := false
		for i := 0; i < MasterIpAttempts; i++ {
			masterTag := ""
			for _, serverPool := range applyCluster.ServerPools {
				if serverPool.Type == cluster.ServerPoolType_Master {
					masterTag = serverPool.Name
				}
			}
			if masterTag == "" {
				return nil, fmt.Errorf("Unable to find master tag for master IP")
			}
			droplets, _, err := Sdk.Client.Droplets.ListByTag(context.TODO(), masterTag, &godo.ListOptions{})
			if err != nil {
				logger.Debug("Hanging for master IP..")
				time.Sleep(time.Duration(MasterIpSleepSecondsPerAttempt) * time.Second)
				continue
			}
			ld := len(droplets)
			if ld == 0 {
				logger.Debug("Hanging for master IP..")
				time.Sleep(time.Duration(MasterIpSleepSecondsPerAttempt) * time.Second)
				continue
			}
			if ld > 1 {
				return nil, fmt.Errorf("Found [%d] droplets for tag [%s]", ld, masterTag)
			}
			droplet := droplets[0]
			masterIpPrivate, err = droplet.PrivateIPv4()
			if err != nil {
				return nil, fmt.Errorf("Unable to detect private IP: %v", err)
			}
			masterIpPublic, err = droplet.PublicIPv4()
			if err != nil {
				return nil, fmt.Errorf("Unable to detect public IP: %v", err)
			}
			found = true
			applyCluster.Values.ItemMap["INJECTEDMASTER"] = fmt.Sprintf("%s:%s", masterIpPrivate, applyCluster.KubernetesApi.Port)
			break
		}
		if !found {
			return nil, fmt.Errorf("Unable to find Master IP after defined wait")
		}
	}

	applyCluster.Values.ItemMap["INJECTEDNAME"] = applyCluster.Name
	applyCluster.Values.ItemMap["INJECTEDPORT"] = applyCluster.KubernetesApi.Port
	userData, err = bootstrap.Inject(userData, applyCluster.Values.ItemMap)
	if err != nil {
		return nil, err
	}

	sshId, err := strconv.Atoi(applyCluster.Ssh.Identifier)
	if err != nil {
		return nil, err
	}
	createRequest := &godo.DropletCreateRequest{
		Name:   expected.(*Droplet).Name,
		Region: expected.(*Droplet).Region,
		Size:   expected.(*Droplet).Size,
		Image: godo.DropletCreateImage{
			Slug: expected.(*Droplet).Image,
		},
		Tags:              []string{expected.(*Droplet).Name},
		PrivateNetworking: true,
		SSHKeys: []godo.DropletCreateSSHKey{
			{
				ID:          sshId,
				Fingerprint: expected.(*Droplet).SShFingerprint,
			},
		},
		UserData: string(userData),
	}
	droplet, _, err := Sdk.Client.Droplets.Create(context.TODO(), createRequest)
	if err != nil {
		return nil, err
	}

	logger.Info("Created Droplet [%d]", droplet.ID)
	id := strconv.Itoa(droplet.ID)
	newResource := &Droplet{
		Shared: Shared{
			Name:    droplet.Name,
			CloudID: id,
		},
		Image:  droplet.Image.Slug,
		Size:   droplet.Size.Slug,
		Region: droplet.Region.Name,
		Count:  expected.(*Droplet).Count,
	}
	applyCluster.KubernetesApi.Endpoint = masterIpPublic
	return newResource, nil
}
func (r *Droplet) Delete(actual cloud.Resource, known *cluster.Cluster) error {
	logger.Debug("droplet.Delete")
	deleteResource := actual.(*Droplet)
	if deleteResource.Name == "" {
		return fmt.Errorf("Unable to delete droplet resource without Name [%s]", deleteResource.Name)
	}

	droplets, _, err := Sdk.Client.Droplets.ListByTag(context.TODO(), r.Name, &godo.ListOptions{})
	if err != nil {
		return err
	}
	ld := len(droplets)
	if ld != 1 {
		return fmt.Errorf("Found [%d] Droplets for Name [%s]", ld, r.Name)
	}
	droplet := droplets[0]
	_, err = Sdk.Client.Droplets.Delete(context.TODO(), droplet.ID)
	if err != nil {
		return err
	}
	logger.Info("Deleted Droplet [%d]", droplet.ID)
	return nil
}

func (r *Droplet) Render(renderResource cloud.Resource, renderCluster *cluster.Cluster) (*cluster.Cluster, error) {
	logger.Debug("droplet.Render")

	serverPool := &cluster.ServerPool{}
	serverPool.Type = r.ServerPool.Type
	serverPool.Image = renderResource.(*Droplet).Image
	serverPool.Size = renderResource.(*Droplet).Size
	serverPool.Name = renderResource.(*Droplet).Name
	serverPool.MaxCount = renderResource.(*Droplet).Count
	found := false
	for i := 0; i < len(renderCluster.ServerPools); i++ {
		if renderCluster.ServerPools[i].Name == renderResource.(*Droplet).Name {
			renderCluster.ServerPools[i].Image = renderResource.(*Droplet).Image
			renderCluster.ServerPools[i].Size = renderResource.(*Droplet).Size
			renderCluster.ServerPools[i].MaxCount = renderResource.(*Droplet).Count
			found = true
		}
	}
	if !found {
		renderCluster.ServerPools = append(renderCluster.ServerPools, serverPool)
	}
	renderCluster.Location = renderResource.(*Droplet).Region
	return renderCluster, nil
}

func (r *Droplet) Tag(tags map[string]string) error {
	return nil
}
