/**
 * Provider setup configuration.
 * Accepts provider type and credentials, stores them for the gateway session.
 */

export type ProviderType = "telegram" | "discord" | "feishu" | "dummy";

export interface ProviderConfig {
  provider: ProviderType;
  token?: string;
  webhook_secret?: string;
  configured_at: string;
}

export class ProviderSetupStore {
  private config: ProviderConfig | null = null;

  configure(provider: ProviderType, token?: string, webhookSecret?: string): ProviderConfig {
    this.config = {
      provider,
      token,
      webhook_secret: webhookSecret,
      configured_at: new Date().toISOString(),
    };

    if (provider === "dummy") {
      console.log(`[provider-setup] dummy provider configured at ${this.config.configured_at}`);
    }

    return this.config;
  }

  getConfig(): ProviderConfig | null {
    return this.config;
  }

  isConfigured(): boolean {
    return this.config !== null;
  }

  getProvider(): ProviderType | null {
    return this.config?.provider ?? null;
  }
}
