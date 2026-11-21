package main

import (
	"runtimeconditions.io/x/rc/common"
	"runtimeconditions.io/x/rc/env"
)

// declaration states what the request logger needs. The calls do nothing when
// the app runs; the Runtime Conditions profiler reads them from the source.
func declaration() {
	_ = common.Api(
		common.Http{
			Spec: &common.Spec{Format: common.FormatOpenapi, Uri: "catalog://api/default/todos-api", Version: &[]string{"1.0.0"}[0]},
			Operations: common.HttpOperations{
				{
					Method: common.MethodGet,
					Path:   "/todos/{id}",
					ResponseSchema: common.ResponseSchemaMap{
						"completed": common.SimpleSchemaStringBoolean,
						"id":        common.SimpleSchemaStringInteger,
						"title":     common.SimpleSchemaStringString,
					},
				},
			},
		},
		env.HttpConfiguration{
			Env: env.HttpConfigurationEnv{
				{Name: "TODOS_API_URL", Property: env.HttpConfigurationEnvPropertyBaseUrl},
			},
		},
	)

	_ = common.Cache(
		common.KeyValue{Engine: &[]common.KeyValueEngine{common.KeyValueEngineRedis}[0]},
		env.KeyValueConfiguration{
			Alternatives: env.KeyValueConfigurationAlternatives{
				{Env: env.KeyValueConfigurationAlternativesEnv{
					{Name: "REDIS_URL", Property: env.KeyValueConfigurationAlternativesEnvPropertyUrl},
				}},
				{Env: env.KeyValueConfigurationAlternativesEnv{
					{Name: "REDIS_HOST", Property: env.KeyValueConfigurationAlternativesEnvPropertyHostname},
					{Name: "REDIS_PORT", Property: env.KeyValueConfigurationAlternativesEnvPropertyPort},
				}},
			},
		},
	)
}
