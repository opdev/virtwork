// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package workloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/opdev/virtwork/internal/config"
	"github.com/opdev/virtwork/internal/workloads"
)

var _ = Describe("ParamSchema", func() {
	var schema workloads.ParamSchema

	BeforeEach(func() {
		schema = workloads.ParamSchema{
			{Key: "load", Type: workloads.ParamInt, Default: "100", Desc: "load percent"},
			{Key: "method", Type: workloads.ParamString, Default: "all", Desc: "stressor method"},
			{Key: "verbose", Type: workloads.ParamBool, Default: "false", Desc: "enable verbose"},
			{Key: "targets", Type: workloads.ParamList, Default: "a;b;c", Desc: "target list"},
			{Key: "labels", Type: workloads.ParamDict, Default: "k1=v1;k2=v2", Desc: "label map"},
		}
	})

	Describe("Find", func() {
		It("returns the ParamDef for a known key", func() {
			def := schema.Find("load")
			Expect(def).NotTo(BeNil())
			Expect(def.Key).To(Equal("load"))
			Expect(def.Type).To(Equal(workloads.ParamInt))
			Expect(def.Default).To(Equal("100"))
		})

		It("returns nil for an unknown key", func() {
			Expect(schema.Find("nonexistent")).To(BeNil())
		})

		It("finds each type correctly", func() {
			Expect(schema.Find("method").Type).To(Equal(workloads.ParamString))
			Expect(schema.Find("verbose").Type).To(Equal(workloads.ParamBool))
			Expect(schema.Find("targets").Type).To(Equal(workloads.ParamList))
			Expect(schema.Find("labels").Type).To(Equal(workloads.ParamDict))
		})
	})

	Describe("Validate", func() {
		Context("ParamString", func() {
			It("accepts a non-empty string", func() {
				def := schema.Find("method")
				Expect(def.Validate("matrixprod")).To(Succeed())
			})

			It("rejects an empty string", func() {
				def := schema.Find("method")
				Expect(def.Validate("")).To(MatchError(ContainSubstring("must not be empty")))
			})
		})

		Context("ParamInt", func() {
			It("accepts a valid integer", func() {
				def := schema.Find("load")
				Expect(def.Validate("42")).To(Succeed())
			})

			It("accepts zero", func() {
				def := schema.Find("load")
				Expect(def.Validate("0")).To(Succeed())
			})

			It("accepts negative integers", func() {
				def := schema.Find("load")
				Expect(def.Validate("-1")).To(Succeed())
			})

			It("rejects non-numeric strings", func() {
				def := schema.Find("load")
				Expect(def.Validate("banana")).To(MatchError(ContainSubstring("must be an integer")))
			})

			It("rejects floats", func() {
				def := schema.Find("load")
				Expect(def.Validate("3.14")).To(MatchError(ContainSubstring("must be an integer")))
			})
		})

		Context("ParamBool", func() {
			It("accepts true (lowercase)", func() {
				def := schema.Find("verbose")
				Expect(def.Validate("true")).To(Succeed())
			})

			It("accepts false (lowercase)", func() {
				def := schema.Find("verbose")
				Expect(def.Validate("false")).To(Succeed())
			})

			It("accepts TRUE (uppercase)", func() {
				def := schema.Find("verbose")
				Expect(def.Validate("TRUE")).To(Succeed())
			})

			It("accepts False (mixed case)", func() {
				def := schema.Find("verbose")
				Expect(def.Validate("False")).To(Succeed())
			})

			It("rejects non-boolean strings", func() {
				def := schema.Find("verbose")
				Expect(def.Validate("yes")).To(MatchError(ContainSubstring("must be \"true\" or \"false\"")))
			})

			It("rejects numeric boolean equivalents", func() {
				def := schema.Find("verbose")
				Expect(def.Validate("1")).To(MatchError(ContainSubstring("must be \"true\" or \"false\"")))
			})
		})

		Context("ParamList", func() {
			It("accepts a single element", func() {
				def := schema.Find("targets")
				Expect(def.Validate("alpha")).To(Succeed())
			})

			It("accepts semicolon-separated elements", func() {
				def := schema.Find("targets")
				Expect(def.Validate("a;b;c")).To(Succeed())
			})

			It("rejects empty elements", func() {
				def := schema.Find("targets")
				Expect(def.Validate("a;;c")).To(MatchError(ContainSubstring("empty element")))
			})

			It("rejects a trailing semicolon", func() {
				def := schema.Find("targets")
				Expect(def.Validate("a;b;")).To(MatchError(ContainSubstring("empty element")))
			})

			It("rejects an empty string", func() {
				def := schema.Find("targets")
				Expect(def.Validate("")).To(MatchError(ContainSubstring("empty element")))
			})
		})

		Context("ParamDict", func() {
			It("accepts a single key=value pair", func() {
				def := schema.Find("labels")
				Expect(def.Validate("env=prod")).To(Succeed())
			})

			It("accepts multiple semicolon-separated pairs", func() {
				def := schema.Find("labels")
				Expect(def.Validate("k1=v1;k2=v2")).To(Succeed())
			})

			It("accepts values containing equals signs", func() {
				def := schema.Find("labels")
				Expect(def.Validate("expr=a=b")).To(Succeed())
			})

			It("rejects entries without an equals sign", func() {
				def := schema.Find("labels")
				Expect(def.Validate("noequals")).To(MatchError(ContainSubstring("must contain \"=\"")))
			})

			It("rejects mixed valid and invalid entries", func() {
				def := schema.Find("labels")
				Expect(def.Validate("k1=v1;bad")).To(MatchError(ContainSubstring("must contain \"=\"")))
			})

			It("rejects empty elements", func() {
				def := schema.Find("labels")
				Expect(def.Validate("k1=v1;;k2=v2")).To(MatchError(ContainSubstring("empty element")))
			})
		})
	})
})

var _ = Describe("GetParam", func() {
	var schema workloads.ParamSchema

	BeforeEach(func() {
		schema = workloads.ParamSchema{
			{Key: "load", Type: workloads.ParamInt, Default: "100", Desc: "load percent"},
			{Key: "method", Type: workloads.ParamString, Default: "all", Desc: "stressor method"},
		}
	})

	It("returns the default when Params is nil", func() {
		w := workloads.BaseWorkload{
			Config:      config.WorkloadConfig{},
			ParamSchema: schema,
		}
		Expect(w.GetParam("load")).To(Equal("100"))
		Expect(w.GetParam("method")).To(Equal("all"))
	})

	It("returns the default when Params is empty", func() {
		w := workloads.BaseWorkload{
			Config:      config.WorkloadConfig{Params: map[string]string{}},
			ParamSchema: schema,
		}
		Expect(w.GetParam("load")).To(Equal("100"))
	})

	It("returns the user value when set", func() {
		w := workloads.BaseWorkload{
			Config: config.WorkloadConfig{
				Params: map[string]string{"load": "50", "method": "matrixprod"},
			},
			ParamSchema: schema,
		}
		Expect(w.GetParam("load")).To(Equal("50"))
		Expect(w.GetParam("method")).To(Equal("matrixprod"))
	})

	It("returns the default for keys not present in Params", func() {
		w := workloads.BaseWorkload{
			Config: config.WorkloadConfig{
				Params: map[string]string{"load": "75"},
			},
			ParamSchema: schema,
		}
		Expect(w.GetParam("load")).To(Equal("75"))
		Expect(w.GetParam("method")).To(Equal("all"))
	})

	It("returns the default when a param value is empty string", func() {
		w := workloads.BaseWorkload{
			Config: config.WorkloadConfig{
				Params: map[string]string{"load": ""},
			},
			ParamSchema: schema,
		}
		Expect(w.GetParam("load")).To(Equal("100"))
	})

	It("panics on an unknown key", func() {
		w := workloads.BaseWorkload{
			Config:      config.WorkloadConfig{},
			ParamSchema: schema,
		}
		Expect(func() { w.GetParam("nonexistent") }).To(PanicWith(ContainSubstring("unknown param key")))
	})
})
