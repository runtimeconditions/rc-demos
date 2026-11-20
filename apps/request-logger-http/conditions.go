package main

import (
	"runtimeconditions.io/x/rc/common"
	"runtimeconditions.io/x/rc/env"
)

// Generated binding declarations are inert; the profiler reads their typed values.
func declaration() {
	common.Api(
		common.Http{
			Spec: &common.Spec{Format: common.FormatOpenapi, Uri: "catalog://api/default/todos-api", Version: &[]string{"1.0.0"}[0]},
			Operations: common.Operations{{Method: common.MethodGet, Path: "/todos/{id}", ResponseSchema: common.ResponseSchemaMap{
				"completed": common.SimpleSchemaString("boolean"), "id": common.SimpleSchemaString("integer"), "title": common.SimpleSchemaString("string"),
			}}},
		},
		env.HttpConfiguration{Env: env.HttpConfigurationEnv{{Name: "TODOS_API_URL", Property: env.HttpConfigurationEnvPropertyBaseUrl}}},
	)
	common.Cache(
		common.KeyValue{Engine: &[]common.KeyValueEngine{common.KeyValueEngineRedis}[0]},
		env.KeyValueConfiguration{Alternatives: env.KeyValueConfigurationAlternatives{
			{Env: env.KeyValueConfigurationAlternativesEnv{{Name: "REDIS_URL", Property: env.KeyValueConfigurationAlternativesEnvPropertyUrl}}},
			{Env: env.KeyValueConfigurationAlternativesEnv{
				{Name: "REDIS_HOST", Property: env.KeyValueConfigurationAlternativesEnvPropertyHostname},
				{Name: "REDIS_PORT", Property: env.KeyValueConfigurationAlternativesEnvPropertyPort},
			}},
		}},
	)
}
