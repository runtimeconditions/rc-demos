import { Entity } from '@backstage/catalog-model';
import {
  createPlugin,
  createRouteRef,
  createRoutableExtension,
} from '@backstage/core-plugin-api';
import { KUBERNETES_ANNOTATION } from '@backstage/plugin-kubernetes-common';

export const rootRouteRef = createRouteRef({ id: 'runtime-conditions' });

export const runtimeConditionsPlugin = createPlugin({
  id: 'runtime-conditions',
  routes: { entityContent: rootRouteRef },
});

export const EntityRuntimeConditionsContent = runtimeConditionsPlugin.provide(
  createRoutableExtension({
    name: 'EntityRuntimeConditionsContent',
    component: () =>
      import('./components/RuntimeConditionsContent').then(
        m => m.RuntimeConditionsContent,
      ),
    mountPoint: rootRouteRef,
  }),
);

export const isRuntimeConditionsAvailable = (entity: Entity) =>
  Boolean(entity.metadata.annotations?.[KUBERNETES_ANNOTATION]);
