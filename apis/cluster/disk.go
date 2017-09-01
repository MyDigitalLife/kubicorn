package cluster

type Disk struct {
	BootDisk bool   `json:"bootdisk,omitempty"`
	SizeGb   int64  `json:"sizegb,omitempty"`
	Image    string `json:"image,omitempty"`
}
