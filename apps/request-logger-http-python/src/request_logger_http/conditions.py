from runtimeconditions_common_integrations import (
    Format,
    Http,
    HttpOperationsItem,
    KeyValue,
    KeyValueEngine,
    Method,
    Spec,
    api,
    cache,
)
from runtimeconditions_env_configuration import (
    HttpConfiguration,
    HttpConfigurationEnvItem,
    HttpConfigurationEnvItemProperty,
    KeyValueConfiguration,
    KeyValueConfigurationAlternativesItem,
    KeyValueConfigurationAlternativesItemEnvItem,
    KeyValueConfigurationAlternativesItemEnvItemProperty,
)

api(
    Http(
        spec=Spec(format=Format.OPENAPI, uri="catalog://api/default/todos-api", version="1.0.0"),
        operations=[HttpOperationsItem(method=Method.GET, path="/todos/{id}")],
    ),
    HttpConfiguration(
        env=[
            HttpConfigurationEnvItem(
                name="TODOS_API_URL",
                property=HttpConfigurationEnvItemProperty.BASE_URL,
            )
        ]
    ),
)

cache(
    KeyValue(engine=KeyValueEngine.REDIS),
    KeyValueConfiguration(
        alternatives=[
            KeyValueConfigurationAlternativesItem(
                env=[
                    KeyValueConfigurationAlternativesItemEnvItem(
                        name="REDIS_URL",
                        property=KeyValueConfigurationAlternativesItemEnvItemProperty.URL,
                    )
                ]
            ),
            KeyValueConfigurationAlternativesItem(
                env=[
                    KeyValueConfigurationAlternativesItemEnvItem(
                        name="REDIS_HOST",
                        property=KeyValueConfigurationAlternativesItemEnvItemProperty.HOSTNAME,
                    ),
                    KeyValueConfigurationAlternativesItemEnvItem(
                        name="REDIS_PORT",
                        property=KeyValueConfigurationAlternativesItemEnvItemProperty.PORT,
                    ),
                ]
            ),
        ]
    ),
)
