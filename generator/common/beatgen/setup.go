package beatgen

import (
	"fmt"
	"os"
	"path/filepath"

	devtools "github.com/elastic/beats/dev-tools/mage"
	"github.com/magefile/mage/sh"
	"github.com/pkg/errors"
)

// RunSetup runs any remaining setup commands after the vendor directory has been setup
func RunSetup() error {
	vendorPath := "./vendor/github.com/"

	//Copy mage stuff
	err := os.MkdirAll(filepath.Join(vendorPath, "magefile"), 0755)
	if err != nil {
		return errors.Wrap(err, "error making mage dir")
	}

	err = sh.Run("cp", "-R", filepath.Join(vendorPath, "elastic/beats/vendor/github.com/magefile/mage"), filepath.Join(vendorPath, "magefile"))
	if err != nil {
		return errors.Wrap(err, "error copying vendored magefile")
	}

	//Copy the pkg helper
	err = sh.Run("cp", "-R", filepath.Join(vendorPath, "elastic/beats/vendor/github.com/pkg"), vendorPath)
	if err != nil {
		return errors.Wrap(err, "error copying pkg to vendor")
	}

	return sh.Run("ln", "-sf", filepath.Join(vendorPath, "elastic/beats/metricbeat/scripts/generate_imports_helper.py"))

}

// CopyVendor copies a new version of beats into the vendor directory of PWD
// By default this uses git archive, meaning any uncommitted changes will not be copied.
// Set the NEWBEAT_DEV env variable to use a slow `cp` copy that will catch uncommited changes
func CopyVendor() error {
	vendorPath := "./vendor/github.com/elastic/"
	beatPath, err := devtools.ElasticBeatsDir()
	if err != nil {
		return errors.Wrap(err, "Could not find ElasticBeatsDir")
	}
	err = os.MkdirAll(vendorPath, 0755)
	if err != nil {
		return errors.Wrap(err, "error creating vendor dir")
	}
	if _, isDev := os.LookupEnv(cfgPrefix + "_DEV"); isDev {
		//Dev mode. Use CP.
		fmt.Printf("CopyVendor unning in dev mode.\n")
		err = sh.Run("cp", "-R", beatPath, vendorPath)
		if err != nil {
			return errors.Wrap(err, "error copying vendor dir")
		}
		err = sh.Rm(filepath.Join(vendorPath, ".git"))
		if err != nil {
			return errors.Wrap(err, "error removing vendor git directory")
		}
		err = sh.Rm(filepath.Join(vendorPath, "x-pack"))
		if err != nil {
			return errors.Wrap(err, "error removing x-pack directory")
		}
	} else {
		//not dev mode. Use git archive
		vendorPath = filepath.Join(vendorPath, "beats")
		err = os.MkdirAll(vendorPath, 0755)
		if err != nil {
			return errors.Wrap(err, "error creating vendor dir")
		}
		err = sh.Run("sh",
			"-c",
			"git archive --remote "+beatPath+" HEAD |  tar -x --exclude=x-pack -C "+vendorPath)
		if err != nil {
			return errors.Wrap(err, "error running git archive")
		}
	}

	return nil

}

// GitInit initializes a new git repo in the current directory
func GitInit() error {
	return sh.Run("git", "init")
}

// GitAdd adds the current working directory to git and does an initial commit
func GitAdd() error {
	err := sh.Run("git", "add", "-A")
	if err != nil {
		return errors.Wrap(err, "error running git add")
	}
	return sh.Run("git", "commit", "-q", "-m", "Initial commit, Add generated files")
}
