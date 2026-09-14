import { useEntity } from '@backstage/plugin-catalog-react';
import { useCustomResources } from '@backstage/plugin-kubernetes-react';
import {
  InfoCard,
  Table,
  Progress,
  ResponseErrorPanel,
} from '@backstage/core-components';

const matchers = [
  {
    group: 'runtimeconditions.io',
    apiVersion: 'v1alpha1',
    plural: 'runtimeconditionsprofiles',
  },
];

export const RuntimeConditionsContent = () => {
  const { entity } = useEntity();
  const { kubernetesObjects, loading, error } = useCustomResources(
    entity,
    matchers,
  );

  if (loading) return <Progress />;
  if (error) return <ResponseErrorPanel error={new Error(error)} />;

  const profile = kubernetesObjects?.items
    .flatMap(item => item.resources)
    .flatMap(response => response.resources)[0];

  if (!profile) {
    return (
      <InfoCard title="Runtime Conditions">
        No RuntimeConditionsProfile found for this component.
      </InfoCard>
    );
  }

  return (
    <InfoCard
      title={`Runtime Conditions: ${profile.metadata.name}`}
      subheader={profile.workload.uri}
    >
      <p>Extensions: {profile.extensions.join(', ')}</p>
      <Table
        options={{ paging: false, search: false }}
        columns={[
          { title: 'Name', field: 'name' },
          { title: 'Kind', field: 'kind' },
          { title: 'Interface', field: 'interfaceType' },
          { title: 'Optional', field: 'optional' },
        ]}
        data={profile.conditions.map((condition: any) => ({
          name: condition.name,
          kind: condition.kind,
          interfaceType: condition.interface?.type,
          optional: condition.optional ? 'yes' : 'no',
        }))}
      />
    </InfoCard>
  );
};
