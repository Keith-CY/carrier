import { createHostManageCodeAgentActions } from './hostManageCodeAgentActions';
import { createHostManageHostActions } from './hostManageHostActions';
import { createHostManageMutationActions } from './hostManageMutationActions';
import { createHostManageReadActions } from './hostManageReadActions';
import { createHostManageRuntime, type HostManageOperationContext } from './hostManageShared';

export function useHostManageOperations(args: HostManageOperationContext) {
  const runtime = createHostManageRuntime(args);

  return {
    ...createHostManageHostActions(args, runtime),
    ...createHostManageReadActions(args, runtime),
    ...createHostManageMutationActions(args, runtime),
    ...createHostManageCodeAgentActions(args, runtime),
  };
}
