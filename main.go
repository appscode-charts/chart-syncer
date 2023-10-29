package main

import (
	"encoding/json"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	flag "github.com/spf13/pflag"
	shell "gomodules.xyz/go-sh"
	"k8s.io/klog/v2"
)

type SearchResult struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	AppVersion  string `json:"app_version"`
	Description string `json:"description"`
}

func main() {
	alias := flag.StringP("alias", "a", "", "Chart registry alias")
	url := flag.StringP("url", "u", "", "Chart registry url from where charts will be downloaded")
	name := flag.StringP("chart", "c", "", "Chart name")
	registry := flag.StringP("registry", "r", "", "OCI registry where images will be uploaded")
	flag.Parse()

	sh := shell.NewSession()
	sh.SetDir("/tmp")

	sh.ShowCMD = true

	// helm repo add fluxcd-community https://fluxcd-community.github.io/helm-charts
	err := sh.Command("helm", "repo", "add", *alias, *url).Run()
	if err != nil {
		// don't exit, because repo might already exist and return error
		klog.Errorln(err)
	}

	// helm search repo fluxcd-community/flux2 -l -o json
	fullname := *alias + "/" + *name
	out, err := sh.Command("helm", "search", "repo", fullname, "-l", "-o", "json").Output()
	if err != nil {
		// don't exit, because repo might already exist and return error
		klog.Errorln(err)
	}
	var results []SearchResult
	err = json.Unmarshal(out, &results)
	if err != nil {
		panic(err)
	}
	for _, result := range results {
		if result.Name != fullname {
			continue
		}
		fmt.Println(fullname, result.Version)

		_, found := ImageDigest(*registry, *name, result.Version)
		if found {
			klog.Infof("skipping syncing %s/%s:%s", *registry, *name, result.Version)
			continue
		}

		// helm pull appscode/ace-installer --version=v2023.03.23
		err := sh.Command("helm", "pull", fullname, "--version", result.Version).Run()
		if err != nil {
			panic(err)
		}

		// flux2-2.10.6.tgz
		filename := fmt.Sprintf("%s-%s.tgz", *name, result.Version)

		// helm push flux2-2.10.6.tgz oci://ghcr.io/gh-walker
		err = sh.Command("helm", "push", filename, fmt.Sprintf("oci://%s/%s", *registry, *name)).Run()
		if err != nil {
			panic(err)
		}

		// helm pull oci://ghcr.io/gh-walker/flux2 --version 2.10.6
	}
}

func ImageDigest(registry, name, version string) (string, bool) {
	// crane digest ghcr.io/gh-walker/flux2:2.10.6
	digest, err := crane.Digest(fmt.Sprintf("%s/%s:%s", registry, name, version), crane.WithAuthFromKeychain(authn.DefaultKeychain))
	if err == nil {
		return digest, true
	}
	klog.Errorln(err)
	return "", false
}
