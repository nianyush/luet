// Copyright © 2019 Ettore Di Giacinto <mudler@gentoo.org>
//
// This program is free software; you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation; either version 2 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along
// with this program; if not, see <http://www.gnu.org/licenses/>.

package installer

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"

	"github.com/ghodss/yaml"
	compiler "github.com/mudler/luet/pkg/compiler"
	"github.com/mudler/luet/pkg/helpers"
	. "github.com/mudler/luet/pkg/logger"
	pkg "github.com/mudler/luet/pkg/package"
	"github.com/mudler/luet/pkg/solver"
	"github.com/mudler/luet/pkg/tree"

	"github.com/pkg/errors"
)

type LuetInstaller struct {
	PackageRepositories Repositories
	Concurrency         int
}

type ArtifactMatch struct {
	Package    pkg.Package
	Artifact   compiler.Artifact
	Repository Repository
}

type LuetFinalizer struct {
	Install   []string `json:"install"`
	Uninstall []string `json:"uninstall"` // TODO: Where to store?
}

func (f *LuetFinalizer) RunInstall() error {
	for _, c := range f.Install {
		Debug("finalizer:", "sh", "-c", c)
		cmd := exec.Command("sh", "-c", c)
		stdoutStderr, err := cmd.CombinedOutput()
		if err != nil {
			return errors.Wrap(err, "Failed running command: "+string(stdoutStderr))
		}
		Info(stdoutStderr)
	}
	return nil
}

// TODO: We don't store uninstall finalizers ?!
func (f *LuetFinalizer) RunUnInstall() error {
	for _, c := range f.Install {
		Debug("finalizer:", "sh", "-c", c)
		cmd := exec.Command("sh", "-c", c)
		stdoutStderr, err := cmd.CombinedOutput()
		if err != nil {
			return errors.Wrap(err, "Failed running command: "+string(stdoutStderr))
		}
		Info(stdoutStderr)
	}
	return nil
}

func NewLuetFinalizerFromYaml(data []byte) (*LuetFinalizer, error) {
	var p LuetFinalizer
	err := yaml.Unmarshal(data, &p)
	if err != nil {
		return &p, err
	}
	return &p, err
}

func NewLuetInstaller(concurrency int) Installer {
	return &LuetInstaller{Concurrency: concurrency}
}

func (l *LuetInstaller) Install(p []pkg.Package, s *System) error {
	// First get metas from all repos (and decodes trees)

	Spinner(32)
	defer SpinnerStop()
	syncedRepos := Repositories{}
	for _, r := range l.PackageRepositories {
		repo, err := r.Sync()
		if err != nil {
			return errors.Wrap(err, "Failed syncing repository: "+r.GetName())
		}
		syncedRepos = append(syncedRepos, repo)
	}

	// compute what to install and from where
	sort.Sort(syncedRepos)

	// First match packages against repositories by priority
	//	matches := syncedRepos.PackageMatches(p)

	// Get installed definition
	installed, err := s.World()
	if err != nil {
		return errors.Wrap(err, "Failed generating installed world")
	}

	// compute a "big" world
	allrepoWorld := syncedRepos.World()

	// If installed exists in the world, we need to use them to make the solver point to them
	realInstalled := []pkg.Package{}
	for _, i := range installed {
		var found pkg.Package
	I:
		for _, p := range allrepoWorld {
			if p.Matches(i) {
				found = p
				break I
			}
		}

		if found != nil {
			realInstalled = append(realInstalled, found)

		} else {
			realInstalled = append(realInstalled, i)
		}
	}

	allwanted := []pkg.Package{}
	for _, wanted := range p {
		var found pkg.Package

	W:
		for _, p := range allrepoWorld {
			if p.Matches(wanted) {
				found = p
				break W
			}
		}

		if found != nil {
			allwanted = append(allwanted, found)

		} else {
			return errors.New("Package requested to install not found")
		}
	}

	solv := solver.NewSolver(realInstalled, allrepoWorld, pkg.NewInMemoryDatabase(false))
	solution, err := solv.Install(allwanted)

	// Gathers things to install
	toInstall := map[string]ArtifactMatch{}
	for _, assertion := range solution {
		if assertion.Value && assertion.Package.Flagged() {
			matches := syncedRepos.PackageMatches([]pkg.Package{assertion.Package})
			if len(matches) != 1 {
				return errors.New("Failed matching solutions against repository - where are definitions coming from?!")
			}
		A:
			for _, artefact := range matches[0].Repo.GetIndex() {
				if artefact.GetCompileSpec().GetPackage() == nil {
					return errors.New("Package in compilespec empty")

				}
				if matches[0].Package.Matches(artefact.GetCompileSpec().GetPackage()) {
					// TODO: Filter out already installed?
					toInstall[assertion.Package.GetFingerPrint()] = ArtifactMatch{Package: assertion.Package, Artifact: artefact, Repository: matches[0].Repo}
					break A
				}
			}
		}
	}

	// Install packages into rootfs in parallel.
	all := make(chan ArtifactMatch)

	var wg = new(sync.WaitGroup)
	for i := 0; i < l.Concurrency; i++ {
		wg.Add(1)
		go l.installerWorker(i, wg, all, s)
	}

	for _, c := range toInstall {
		all <- c
	}
	close(all)
	wg.Wait()

	executedFinalizer := map[string]bool{}

	// TODO: Lower those errors as warning
	for _, w := range allwanted {
		// Finalizers needs to run in order and in sequence.
		ordered := solution.Order(w.GetFingerPrint())
		for _, ass := range ordered {
			if ass.Value && ass.Package.Flagged() {
				// Annotate to the system that the package was installed
				// TODO: Annotate also files that belong to the package, somewhere to uninstall
				if _, err := s.Database.FindPackage(ass.Package); err == nil {
					err := s.Database.UpdatePackage(ass.Package)
					if err != nil {
						return errors.Wrap(err, "Failed updating package")
					}
				} else {
					_, err := s.Database.CreatePackage(ass.Package)
					if err != nil {
						return errors.Wrap(err, "Failed creating package")
					}
				}
				installed, ok := toInstall[ass.Package.GetFingerPrint()]
				if !ok {
					return errors.New("Couldn't find ArtifactMatch for " + ass.Package.GetFingerPrint())
				}

				treePackage, err := installed.Repository.GetTree().Tree().FindPackage(ass.Package)
				if err != nil {
					return errors.Wrap(err, "Error getting package "+ass.Package.GetFingerPrint())
				}
				if helpers.Exists(treePackage.Rel(tree.FinalizerFile)) {
					Info("Executing finalizer for " + ass.Package.GetName())
					finalizerRaw, err := ioutil.ReadFile(treePackage.Rel(tree.FinalizerFile))
					if err != nil {
						return errors.Wrap(err, "Error reading file "+treePackage.Rel(tree.FinalizerFile))
					}
					if _, exists := executedFinalizer[ass.Package.GetFingerPrint()]; !exists {
						finalizer, err := NewLuetFinalizerFromYaml(finalizerRaw)
						if err != nil {
							return errors.Wrap(err, "Error reading finalizer "+treePackage.Rel(tree.FinalizerFile))
						}
						err = finalizer.RunInstall()
						if err != nil {
							return errors.Wrap(err, "Error executing install finalizer "+treePackage.Rel(tree.FinalizerFile))
						}
						executedFinalizer[ass.Package.GetFingerPrint()] = true
					}
				}
			}
		}

	}

	return nil

}

func (l *LuetInstaller) installPackage(a ArtifactMatch, s *System) error {

	// FIXME: Implement
	artifact, err := a.Repository.Client().DownloadArtifact(a.Artifact)
	defer os.Remove(artifact.GetPath())

	tarFile, err := os.Open(artifact.GetPath())
	if err != nil {
		return errors.Wrap(err, "Could not open package archive")
	}
	defer tarFile.Close()
	tr := tar.NewReader(tarFile)

	var files []string
	// untar each segment
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// determine proper file path info
		finfo := hdr.FileInfo()
		fileName := hdr.Name
		if finfo.Mode().IsDir() {
			continue
		}
		files = append(files, fileName)

		// if a dir, create it, then go to next segment
	}

	err = helpers.Untar(artifact.GetPath(), s.Target, true)
	if err != nil {
		return errors.Wrap(err, "Error met while unpacking rootfs")
	}

	// First create client and download
	// Then unpack to system
	return s.Database.SetPackageFiles(&pkg.PackageFile{PackageFingerprint: a.Package.GetFingerPrint(), Files: files})
}

func (l *LuetInstaller) installerWorker(i int, wg *sync.WaitGroup, c <-chan ArtifactMatch, s *System) error {
	defer wg.Done()

	for p := range c {
		// TODO: Keep trace of what was added from the tar, and save it into system
		err := l.installPackage(p, s)
		if err != nil {
			//TODO: Uninstall, rollback.
			Fatal("Failed installing package "+p.Package.GetName(), err.Error())
			return errors.Wrap(err, "Failed installing package "+p.Package.GetName())
		}
	}

	return nil
}

func (l *LuetInstaller) uninstall(p pkg.Package, s *System) error {
	files, err := s.Database.GetPackageFiles(p)
	if err != nil {
		return errors.Wrap(err, "Failed getting installed files")
	}

	// Remove from target
	for _, f := range files {
		target := filepath.Join(s.Target, f)
		Info("Removing", target)
		err := os.Remove(target)
		if err != nil {
			Warning("Failed removing file (not present in the system target ?)", target)
		}
	}
	err = s.Database.RemovePackageFiles(p)
	if err != nil {
		return errors.Wrap(err, "Failed removing package files from database")
	}
	err = s.Database.RemovePackage(p)
	if err != nil {
		return errors.Wrap(err, "Failed removing package from database")
	}

	Info(p.GetFingerPrint(), "Removed")
	return nil
}

func (l *LuetInstaller) Uninstall(p pkg.Package, s *System) error {
	// compute uninstall from all world - remove packages in parallel - run uninstall finalizer (in order) - mark the uninstallation in db
	// Get installed definition

	selected, err := s.Database.FindPackage(p)
	if err != nil {
		return errors.Wrap(err, "Package not installed")
	}
	installed, err := s.World()
	if err != nil {
		return errors.Wrap(err, "Failed generating installed world")
	}

	solv := solver.NewSolver(installed, installed, pkg.NewInMemoryDatabase(false))
	solution, err := solv.Uninstall(selected)
	for _, p := range solution {
		Info("Uninstalling", p.GetFingerPrint())
		err := l.uninstall(p, s)
		if err != nil {
			return errors.Wrap(err, "Uninstall failed")
		}
	}
	return nil

}

func (l *LuetInstaller) Repositories(r []Repository) { l.PackageRepositories = r }