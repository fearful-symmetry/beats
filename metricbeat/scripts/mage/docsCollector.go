package mage

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/elastic/beats/dev-tools/mage"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// this struct provides module-level data that will be used to populate the module list
type moduleData struct {
	Path       string
	Base       string
	Title      string `yaml:"title"`
	Release    string `yaml:"release"`
	Dashboards bool
	Settings   []string `yaml:"settings"`
	CfgFile    string
	Asciidoc   string
	Metricsets []metricsetData
}

//check to see if the config list has a particular item
func (m moduleData) checkConfig(key string) bool {
	for _, item := range m.Settings {
		if item == key {
			return true
		}
	}
	return false
}

type metricsetData struct {
	Path       string
	Title      string
	Link       string
	Release    string
	DataExists bool
}

var generatedNote = `////
This file is generated! See scripts/docs_collector.py
////

`

var moduleExampleConfig = `

[float]
=== Example configuration

The %s module supports the standard configuration options that are described
in <<configuration-metricbeat>>. Here is an example configuration:

[source,yaml]
----
metricbeat.modules:
`

var metricsetFields = `

==== Fields

For a description of each field in the metricset, see the
<<exported-fields-%s,exported fields>> section.

`

// get the release tag. This kind of logic is needed because any
// module/metricset missing a release is categorized as "experimental"
// There are three valid tags: experimental, beta, ga
func getRelease(rel string) (string, error) {
	if rel != "" {
		if rel != "beta" && rel != "ga" {
			return "", fmt.Errorf("unknown release tag %s", rel)
		}
		return rel, nil
	}
	return "experimental", nil
}

//create the path
func createDocsPath(module string) error {
	return os.MkdirAll(mage.OSSBeatDir(filepath.Join("docs", module)), 0755)
}

//test for a `_meta/docs.asciidoc` in a given directory
func testIfDocsInDir(moduleDir string) (bool, error) {
	moduledir, err := os.Stat(moduleDir)
	if err != nil {
		return false, err
	}
	if moduledir.Mode().IsRegular() {
		return false, nil
	}
	_, err = os.Stat(filepath.Join(moduleDir, "_meta/docs.asciidoc"))
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, errors.Wrap(err, "error looking for asciidoc")
	}
	return true, nil
}

//load the module-specific fields.yml file
func loadModuleFields(file string) (moduleData, error) {
	fd, err := ioutil.ReadFile(file)
	if err != nil {
		return moduleData{}, errors.Wrap(err, "failed to read from spec file")
	}
	//Cheat and use the same struct.
	var mod []moduleData
	if err = yaml.Unmarshal(fd, &mod); err != nil {
		return mod[0], err
	}
	module := mod[0]

	rel, err := getRelease(module.Release)
	if err != nil {
		return mod[0], err
	}
	module.Release = rel

	return module, nil
}

//The doc generator only needs the release field from the metricset-level fields.yml.
func getReleaseState(metricsetPath string) (string, error) {
	raw, err := ioutil.ReadFile(metricsetPath)
	if err != nil {
		return "", errors.Wrap(err, "failed to read from spec file")
	}

	type metricset struct {
		Release string `yaml:"release"`
	}
	var rel []metricset
	if err = yaml.Unmarshal(raw, &rel); err != nil {
		return "", err
	}

	return getRelease(rel[0].Release)
}

//check to see if the metricset has dashboards
func hasDashboards(modulePath string) bool {
	_, err := os.Stat(filepath.Join(modulePath, "_meta/kibana"))
	if err != nil {
		return false
	}

	return true
}

//use the reference file if it exists. if not, the normal one.
func getConfigfile(modulePath string) (string, error) {
	var cfgFile string
	cfg := filepath.Join(modulePath, "_meta/config.reference.yml")
	_, err := os.Stat(cfg)
	if err == nil {
		cfgFile = cfg
	} else if os.IsNotExist(err) {
		cfgBack := filepath.Join(modulePath, "_meta/config.yml")
		if _, err := os.Stat(cfgBack); err != nil {
			return "", fmt.Errorf("config file in %s could not be found", modulePath)
		}
		cfgFile = cfgBack
	} else {
		return "", fmt.Errorf("config file in %s could not be found", modulePath)
	}

	raw, err := ioutil.ReadFile(cfgFile)
	return string(raw), err

}

//gather all the metricsets for a given module
func gatherMetricsets(modulePath string, moduleName string) ([]metricsetData, error) {
	metricsetList, err := filepath.Glob(filepath.Join(modulePath, "/*"))
	if err != nil {
		return nil, err
	}
	var metricsets []metricsetData
	for _, metricset := range metricsetList {
		isMetricset, err := testIfDocsInDir(metricset)
		if err != nil {
			return nil, err
		}
		if !isMetricset {
			continue
		}
		metricsetName := filepath.Base(metricset)
		release, err := getReleaseState(filepath.Join(metricset, "_meta/fields.yml"))
		if err != nil {
			return nil, err
		}

		//generate the asciidoc link used in the module docs, since we need this in a few places
		link := fmt.Sprintf("<<metricbeat-metricset-%s-%s,%s>>", moduleName, metricsetName, metricsetName)

		//test to see if the metricset has a data.json
		hasData := false
		_, err = os.Stat(filepath.Join(metricset, "_meta/data.json"))
		if err == nil {
			hasData = true
		}

		ms := metricsetData{
			Path:       metricset,
			Title:      metricsetName,
			Release:    release,
			Link:       link,
			DataExists: hasData,
		}

		metricsets = append(metricsets, ms)

	} // end of metricset loop

	return metricsets, nil
}

//Gather all the data we need to construct the docs that end up in metricbeat/docs
func gatherData(modules []string) (map[string]moduleData, error) {
	moduleMap := make(map[string]moduleData)
	//iterate over all the modules, checking to make sure we have an asciidoc file
	for _, module := range modules {

		isModule, err := testIfDocsInDir(module)
		if err != nil {
			return moduleMap, err
		}
		if !isModule {
			continue
		}
		moduleName := filepath.Base(module)

		err = createDocsPath(moduleName)
		if err != nil {
			return moduleMap, err
		}

		fieldsm, err := loadModuleFields(filepath.Join(module, "_meta/fields.yml"))
		if err != nil {
			return moduleMap, err
		}

		cfgPath, err := getConfigfile(module)
		if err != nil {
			return moduleMap, err
		}

		metricsets, err := gatherMetricsets(module, moduleName)
		if err != nil {
			return moduleMap, err
		}

		//dump the contents of the module asciidoc
		moduleDoc, err := ioutil.ReadFile(filepath.Join(module, "_meta/docs.asciidoc"))
		if err != nil {
			return moduleMap, err
		}

		fieldsm.Path = module
		fieldsm.CfgFile = cfgPath
		fieldsm.Metricsets = metricsets
		fieldsm.Asciidoc = string(moduleDoc)
		fieldsm.Dashboards = hasDashboards(module)

		moduleMap[moduleName] = fieldsm

	} // end of modules loop

	return moduleMap, nil
}

// Write the module data to docs/
func writeDocs(modules map[string]moduleData) error {

	err := writeModuleDocs(modules)
	if err != nil {
		return errors.Wrap(err, "error writing module docs")
	}
	err = writeMetricsetDocs(modules)
	if err != nil {
		return errors.Wrap(err, "error writing metricset docs")
	}

	err = writeModuleList(modules)
	if err != nil {
		return errors.Wrap(err, "error writing module list")
	}

	return nil
}

// write the module-level docs
func writeModuleDocs(modules map[string]moduleData) error {
	for moduleName, mod := range modules {

		moduleFile := generatedNote
		moduleFile = moduleFile + fmt.Sprintf("[[metricbeat-module-%s]]\n", moduleName)
		moduleFile = moduleFile + fmt.Sprintf("== %s module\n\n", mod.Title)

		if mod.Release != "ga" {
			moduleFile = moduleFile + fmt.Sprintf("%s[]\n\n", mod.Release)
		}

		//Add the asciidoc lines and config header
		moduleFile = moduleFile + mod.Asciidoc + fmt.Sprintf(moduleExampleConfig, mod.Title)
		//Add the config
		moduleFile = moduleFile + mod.CfgFile + "----\n\n"

		//we're doing this in a somewhat klunky way to insure the order of the original python script is preserved
		additonalHelpers := false
		if mod.checkConfig("ssl") {
			moduleFile = moduleFile + "This module supports TLS connections when using `ssl` config field, as described in <<configuration-ssl>>.\n"
			additonalHelpers = true
		}
		if mod.checkConfig("http") {
			moduleFile = moduleFile + "It also supports the options described in <<module-http-config-options>>.\n"
			additonalHelpers = true
		}

		if additonalHelpers {
			moduleFile = moduleFile + "\n"
		}

		//Add the metricset links
		moduleFile = moduleFile + "[float]\n=== Metricsets\n\nThe following metricsets are available:\n\n"
		//iterate over the metricsets, adding links and includes.
		//Again, this particular way is done to preserve the output of the original python script
		for _, ms := range mod.Metricsets {
			moduleFile = moduleFile + fmt.Sprintf("* %s\n\n", ms.Link)
		}
		for _, ms := range mod.Metricsets {
			moduleFile = moduleFile + fmt.Sprintf("include::%s/%s.asciidoc[]\n\n", moduleName, ms.Title)
		}

		//write to doc file
		filename := mage.OSSBeatDir(filepath.Join("docs", "modules", fmt.Sprintf("%s.asciidoc", moduleName)))
		err := ioutil.WriteFile(filename, []byte(moduleFile), 744)
		if err != nil {
			return errors.Wrapf(err, "error writing file %s", filename)
		}
	}

	return nil
}

// write the metricset-level docs
func writeMetricsetDocs(modules map[string]moduleData) error {
	for moduleName, mod := range modules {

		for _, metricset := range mod.Metricsets {
			metricsetFile := generatedNote
			metricsetFile = metricsetFile + fmt.Sprintf("[[metricbeat-metricset-%s-%s]]\n", moduleName, metricset.Title)
			metricsetFile = metricsetFile + fmt.Sprintf("=== %s %s metricset\n\n", mod.Title, metricset.Title)

			if metricset.Release != "ga" {
				metricsetFile = metricsetFile + fmt.Sprintf("%s[]\n\n", metricset.Release)
			}

			//We're doing this because the maage.*Dir() functions will return an absolute path, which we can't just throw into the docs.
			//So emulate the behavior of the python scripts and have relative paths
			base := "module"
			if strings.Contains(mod.Path, mage.XPackBeatDir()) {
				base = "../x-pack/metricbeat/module"
			}

			metricsetFile = metricsetFile + fmt.Sprintf("include::../../../%s/%s/%s/_meta/docs.asciidoc[]\n", base, moduleName, metricset.Title)
			metricsetFile = metricsetFile + fmt.Sprintf(metricsetFields, moduleName)

			if metricset.DataExists {
				metricsetFile = metricsetFile + "Here is an example document generated by this metricset:\n\n[source,json]\n----\n"
				metricsetFile = metricsetFile + fmt.Sprintf("include::../../../%s/%s/%s/_meta/data.json[]\n----\n", base, moduleName, metricset.Title)

			}

			//write to the metricset doc file
			filename := mage.OSSBeatDir(filepath.Join("docs", "modules", moduleName, fmt.Sprintf("%s.asciidoc", metricset.Title)))
			err := ioutil.WriteFile(filename, []byte(metricsetFile), 744)
			if err != nil {
				return errors.Wrapf(err, "error writing file %s", filename)
			}

		} // end metricset loop

	} // end module loop

	return nil
}

func writeModuleList(modules map[string]moduleData) error {
	noIcon := "image:./images/icon-no.png[No prebuilt dashboards] "
	yesIcon := "image:./images/icon-yes.png[Prebuilt dashboards are available] "
	moduleList := generatedNote
	moduleList = moduleList + "[options=\"header\"]\n|===\n|Modules   |Dashboards   |Metricsets   \n"

	//sort the map by sorting the keys, then arrange links in alphabetical order
	keys := make([]string, 0)
	for key := range modules {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		mod := modules[key]

		releaseString := ""
		if mod.Release != "ga" {
			releaseString = fmt.Sprintf("%s[]", mod.Release)
		}

		hasDash := noIcon
		if mod.Dashboards {
			hasDash = yesIcon
		}

		moduleList = moduleList + fmt.Sprintf("|<<metricbeat-module-%s,%s>>  %s   |%s   |  \n", key, mod.Title, releaseString, hasDash)
		//Make sure empty entry row spans over all metricset rows for this module
		moduleList = moduleList + fmt.Sprintf(".%d+| .%d+|  ", len(mod.Metricsets), len(mod.Metricsets))

		//gotta sort these again
		sort.Slice(mod.Metricsets, func(i, j int) bool {
			return mod.Metricsets[i].Title < mod.Metricsets[j].Title
		})
		for _, ms := range mod.Metricsets {
			msReleaseString := ""
			if ms.Release != "ga" {
				msReleaseString = fmt.Sprintf("%s[]", ms.Release)
			}
			moduleList = moduleList + fmt.Sprintf("|%s %s  \n", ms.Link, msReleaseString)
		}

	}
	moduleList = moduleList + "|===\n\n--\n\n"
	//iterate again to add the includes
	for _, key := range keys {
		moduleList = moduleList + fmt.Sprintf("include::modules/%s.asciidoc[]\n", key)
	}

	//write the module list
	filepath := mage.OSSBeatDir(filepath.Join("docs", "modules_list.asciidoc"))
	err := ioutil.WriteFile(filepath, []byte(moduleList), 744)
	if err != nil {
		return errors.Wrapf(err, "error writing file %s", filepath)
	}

	return nil
}

// CollectDocs does the following:
// Generate the module-level docs under docs/
// Generate the module lists
// Generate the metricset-level docs
// All these are 'collected' from the asciidoc files under _meta/ in each module & metricset
func CollectDocs() error {

	//collect modules that have an asciidoc file
	beatsModuleGlob := filepath.Join(mage.OSSBeatDir("module"), "/*/")
	modules, err := filepath.Glob(beatsModuleGlob)
	if err != nil {
		return err
	}

	//collect additional x-pack modules
	xpackModuleGlob := filepath.Join(mage.XPackBeatDir("module"), "/*/")
	xpackModules, err := filepath.Glob(xpackModuleGlob)
	if err != nil {
		return err
	}
	modules = append(modules, xpackModules...)

	moduleMap, err := gatherData(modules)
	if err != nil {
		return err
	}

	writeDocs(moduleMap)

	return nil
}
