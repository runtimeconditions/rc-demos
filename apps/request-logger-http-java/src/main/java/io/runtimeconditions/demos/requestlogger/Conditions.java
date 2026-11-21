package io.runtimeconditions.demos.requestlogger;

import io.runtimeconditions.bindings.commonintegrations.Bindings;
import io.runtimeconditions.bindings.commonintegrations.Format;
import io.runtimeconditions.bindings.commonintegrations.Http;
import io.runtimeconditions.bindings.commonintegrations.HttpOperations;
import io.runtimeconditions.bindings.commonintegrations.HttpOperationsItem;
import io.runtimeconditions.bindings.commonintegrations.KeyValue;
import io.runtimeconditions.bindings.commonintegrations.KeyValueEngine;
import io.runtimeconditions.bindings.commonintegrations.Method;
import io.runtimeconditions.bindings.commonintegrations.Spec;
import io.runtimeconditions.bindings.envconfiguration.HttpConfiguration;
import io.runtimeconditions.bindings.envconfiguration.HttpConfigurationEnv;
import io.runtimeconditions.bindings.envconfiguration.HttpConfigurationEnvItem;
import io.runtimeconditions.bindings.envconfiguration.HttpConfigurationEnvItemProperty;
import io.runtimeconditions.bindings.envconfiguration.KeyValueConfiguration;
import io.runtimeconditions.bindings.envconfiguration.KeyValueConfigurationAlternatives;
import io.runtimeconditions.bindings.envconfiguration.KeyValueConfigurationAlternativesItem;
import io.runtimeconditions.bindings.envconfiguration.KeyValueConfigurationAlternativesItemEnv;
import io.runtimeconditions.bindings.envconfiguration.KeyValueConfigurationAlternativesItemEnvItem;
import io.runtimeconditions.bindings.envconfiguration.KeyValueConfigurationAlternativesItemEnvItemProperty;
import java.util.List;

/**
 * States what the request logger needs. The calls do nothing when the app runs; the Runtime Conditions
 * profiler reads them from the source.
 */
final class Conditions {
    private Conditions() {
    }

    static final Object TODOS_API = Bindings.api(
            new Http(
                    // The response shape (completed, id, title) is left out: the Java profiler cannot yet read the
                    // enum constants that name its field types.
                    new HttpOperations(List.of(new HttpOperationsItem(Method.GET, "/todos/{id}", null, null))),
                    new Spec(Format.OPENAPI, "catalog://api/default/todos-api", "1.0.0")),
            new HttpConfiguration(
                    null,
                    new HttpConfigurationEnv(List.of(new HttpConfigurationEnvItem(
                            "TODOS_API_URL", HttpConfigurationEnvItemProperty.BASE_URL, null, null)))));

    static final Object REQUEST_CACHE = Bindings.cache(
            new KeyValue(KeyValueEngine.REDIS),
            new KeyValueConfiguration(
                    new KeyValueConfigurationAlternatives(List.of(
                            new KeyValueConfigurationAlternativesItem(new KeyValueConfigurationAlternativesItemEnv(List.of(
                                    new KeyValueConfigurationAlternativesItemEnvItem(
                                            "REDIS_URL", KeyValueConfigurationAlternativesItemEnvItemProperty.URL, null, null)))),
                            new KeyValueConfigurationAlternativesItem(new KeyValueConfigurationAlternativesItemEnv(List.of(
                                    new KeyValueConfigurationAlternativesItemEnvItem(
                                            "REDIS_HOST", KeyValueConfigurationAlternativesItemEnvItemProperty.HOSTNAME, null, null),
                                    new KeyValueConfigurationAlternativesItemEnvItem(
                                            "REDIS_PORT", KeyValueConfigurationAlternativesItemEnvItemProperty.PORT, null, null)))))),
                    null));
}
