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

package solver_test

import (
	"strconv"

	pkg "github.com/mudler/luet/pkg/package"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/mudler/luet/pkg/solver"
)

var _ = Describe("Solver Benchmarks", func() {

	db := pkg.NewInMemoryDatabase(false)
	dbInstalled := pkg.NewInMemoryDatabase(false)
	dbDefinitions := pkg.NewInMemoryDatabase(false)
	var s PackageSolver

	Context("Complex data sets", func() {
		BeforeEach(func() {
			db = pkg.NewInMemoryDatabase(false)
			dbInstalled = pkg.NewInMemoryDatabase(false)
			dbDefinitions = pkg.NewInMemoryDatabase(false)
			s = NewSolver(Options{Type: SingleCoreSimple}, dbInstalled, dbDefinitions, db)
		})
		Measure("it should be fast in resolution from a 50000 dataset", func(b Benchmarker) {

			runtime := b.Time("runtime", func() {
				for i := 0; i < 50000; i++ {
					C := pkg.NewPackage("C"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					E := pkg.NewPackage("E"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					F := pkg.NewPackage("F"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					G := pkg.NewPackage("G"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					H := pkg.NewPackage("H"+strconv.Itoa(i), "", []*pkg.DefaultPackage{G}, []*pkg.DefaultPackage{})
					D := pkg.NewPackage("D"+strconv.Itoa(i), "", []*pkg.DefaultPackage{H}, []*pkg.DefaultPackage{})
					B := pkg.NewPackage("B"+strconv.Itoa(i), "", []*pkg.DefaultPackage{D}, []*pkg.DefaultPackage{})
					A := pkg.NewPackage("A"+strconv.Itoa(i), "", []*pkg.DefaultPackage{B}, []*pkg.DefaultPackage{})
					for _, p := range []pkg.Package{A, B, C, D, E, F, G} {
						_, err := dbDefinitions.CreatePackage(p)
						Expect(err).ToNot(HaveOccurred())
					}
					_, err := dbInstalled.CreatePackage(C)
					Expect(err).ToNot(HaveOccurred())
				}

				for i := 0; i < 2; i++ {
					C := pkg.NewPackage("C"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					G := pkg.NewPackage("G"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					H := pkg.NewPackage("H"+strconv.Itoa(i), "", []*pkg.DefaultPackage{G}, []*pkg.DefaultPackage{})
					D := pkg.NewPackage("D"+strconv.Itoa(i), "", []*pkg.DefaultPackage{H}, []*pkg.DefaultPackage{})
					B := pkg.NewPackage("B"+strconv.Itoa(i), "", []*pkg.DefaultPackage{D}, []*pkg.DefaultPackage{})
					A := pkg.NewPackage("A"+strconv.Itoa(i), "", []*pkg.DefaultPackage{B}, []*pkg.DefaultPackage{})

					solution, err := s.Install([]pkg.Package{A})
					Expect(err).ToNot(HaveOccurred())

					Expect(solution).To(ContainElement(PackageAssert{Package: A, Value: true}))
					Expect(solution).To(ContainElement(PackageAssert{Package: B, Value: true}))
					Expect(solution).To(ContainElement(PackageAssert{Package: D, Value: true}))
					Expect(solution).To(ContainElement(PackageAssert{Package: C, Value: true}))
					Expect(solution).To(ContainElement(PackageAssert{Package: H, Value: true}))
					Expect(solution).To(ContainElement(PackageAssert{Package: G, Value: true}))
				}
			})

			Ω(runtime.Seconds()).Should(BeNumerically("<", 120), "Install() shouldn't take too long.")
		}, 5)
	})

	Context("Complex data sets - Parallel", func() {
		BeforeEach(func() {
			db = pkg.NewInMemoryDatabase(false)
			dbInstalled = pkg.NewInMemoryDatabase(false)
			dbDefinitions = pkg.NewInMemoryDatabase(false)
			s = NewSolver(Options{Type: ParallelSimple, Concurrency: 10}, dbInstalled, dbDefinitions, db)
		})
		Measure("it should be fast in resolution from a 50000 dataset", func(b Benchmarker) {
			runtime := b.Time("runtime", func() {
				for i := 0; i < 50000; i++ {
					C := pkg.NewPackage("C"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					E := pkg.NewPackage("E"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					F := pkg.NewPackage("F"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					G := pkg.NewPackage("G"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					H := pkg.NewPackage("H"+strconv.Itoa(i), "", []*pkg.DefaultPackage{G}, []*pkg.DefaultPackage{})
					D := pkg.NewPackage("D"+strconv.Itoa(i), "", []*pkg.DefaultPackage{H}, []*pkg.DefaultPackage{})
					B := pkg.NewPackage("B"+strconv.Itoa(i), "", []*pkg.DefaultPackage{D}, []*pkg.DefaultPackage{})
					A := pkg.NewPackage("A"+strconv.Itoa(i), "", []*pkg.DefaultPackage{B}, []*pkg.DefaultPackage{})
					for _, p := range []pkg.Package{A, B, C, D, E, F, G} {
						_, err := dbDefinitions.CreatePackage(p)
						Expect(err).ToNot(HaveOccurred())
					}
					_, err := dbInstalled.CreatePackage(C)
					Expect(err).ToNot(HaveOccurred())
				}
				for i := 0; i < 2; i++ {
					C := pkg.NewPackage("C"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					G := pkg.NewPackage("G"+strconv.Itoa(i), "", []*pkg.DefaultPackage{}, []*pkg.DefaultPackage{})
					H := pkg.NewPackage("H"+strconv.Itoa(i), "", []*pkg.DefaultPackage{G}, []*pkg.DefaultPackage{})
					D := pkg.NewPackage("D"+strconv.Itoa(i), "", []*pkg.DefaultPackage{H}, []*pkg.DefaultPackage{})
					B := pkg.NewPackage("B"+strconv.Itoa(i), "", []*pkg.DefaultPackage{D}, []*pkg.DefaultPackage{})
					A := pkg.NewPackage("A"+strconv.Itoa(i), "", []*pkg.DefaultPackage{B}, []*pkg.DefaultPackage{})

					solution, err := s.Install([]pkg.Package{A})
					Expect(err).ToNot(HaveOccurred())

					Expect(solution).To(ContainElement(PackageAssert{Package: A, Value: true}))
					Expect(solution).To(ContainElement(PackageAssert{Package: B, Value: true}))
					Expect(solution).To(ContainElement(PackageAssert{Package: D, Value: true}))
					Expect(solution).To(ContainElement(PackageAssert{Package: C, Value: true}))
					Expect(solution).To(ContainElement(PackageAssert{Package: H, Value: true}))
					Expect(solution).To(ContainElement(PackageAssert{Package: G, Value: true}))

					//	Expect(len(solution)).To(Equal(6))
				}
			})

			Ω(runtime.Seconds()).Should(BeNumerically("<", 70), "Install() shouldn't take too long.")
		}, 5)
	})

})
