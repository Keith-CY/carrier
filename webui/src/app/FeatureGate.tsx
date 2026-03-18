import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { useFeatures } from './useFeatures';

type FeatureGateProps = {
  children: ReactNode;
  requireRemoteControlPlane?: boolean;
  requireRemoteChat?: boolean;
  redirectTo?: string;
};

export function FeatureGate({
  children,
  requireRemoteControlPlane = false,
  requireRemoteChat = false,
  redirectTo = '/home',
}: FeatureGateProps) {
  const { featureFlags, isLoading } = useFeatures();

  if (isLoading) return null;
  if (requireRemoteControlPlane && !featureFlags.remoteControlPlaneEnabled) {
    return <Navigate to={redirectTo} replace />;
  }
  if (requireRemoteChat && (!featureFlags.remoteControlPlaneEnabled || !featureFlags.remoteChatEnabled)) {
    return <Navigate to={redirectTo} replace />;
  }
  return <>{children}</>;
}
