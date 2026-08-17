export interface ChatbotSettings {
  enabled: boolean;
  greeting_message: string;
  fallback_message: string;
  session_timeout_minutes: number;
  ai_enabled: boolean;
  ai_provider: string;
}

export interface Stats {
  total_sessions: number;
  active_sessions: number;
  messages_handled: number;
  ai_responses: number;
  agent_transfers: number;
  keywords_count: number;
  flows_count: number;
  ai_contexts_count: number;
}
