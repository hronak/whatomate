-- Migration for Issue 16: WhatsAppAccount as foreign key

-- Update contacts
ALTER TABLE contacts ADD COLUMN whatsapp_account_id UUID;
UPDATE contacts SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE contacts.whatsapp_account = wa.name AND contacts.organization_id = wa.organization_id;
-- It's possible some were null/empty if it was not null. Actually whatsapp_account was string.
ALTER TABLE contacts DROP COLUMN whatsapp_account;
CREATE INDEX idx_contacts_whatsapp_account_id ON contacts(whatsapp_account_id);

-- Update templates
ALTER TABLE templates ADD COLUMN whatsapp_account_id UUID;
UPDATE templates SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE templates.whatsapp_account = wa.name AND templates.organization_id = wa.organization_id;
ALTER TABLE templates DROP COLUMN whatsapp_account;
CREATE INDEX idx_templates_whatsapp_account_id ON templates(whatsapp_account_id);

-- Update whatsapp_flows
ALTER TABLE whatsapp_flows ADD COLUMN whatsapp_account_id UUID;
UPDATE whatsapp_flows SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE whatsapp_flows.whatsapp_account = wa.name AND whatsapp_flows.organization_id = wa.organization_id;
ALTER TABLE whatsapp_flows DROP COLUMN whatsapp_account;
CREATE INDEX idx_whatsapp_flows_whatsapp_account_id ON whatsapp_flows(whatsapp_account_id);

-- Update messages
ALTER TABLE messages ADD COLUMN whatsapp_account_id UUID;
UPDATE messages SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE messages.whatsapp_account = wa.name AND messages.organization_id = wa.organization_id;
ALTER TABLE messages DROP COLUMN whatsapp_account;
CREATE INDEX idx_messages_whatsapp_account_id ON messages(whatsapp_account_id);

-- Update chatbot_settings
ALTER TABLE chatbot_settings ADD COLUMN whatsapp_account_id UUID;
UPDATE chatbot_settings SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE chatbot_settings.whatsapp_account = wa.name AND chatbot_settings.organization_id = wa.organization_id;
ALTER TABLE chatbot_settings DROP COLUMN whatsapp_account;
CREATE INDEX idx_chatbot_settings_whatsapp_account_id ON chatbot_settings(whatsapp_account_id);

-- Update keyword_rules
ALTER TABLE keyword_rules ADD COLUMN whatsapp_account_id UUID;
UPDATE keyword_rules SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE keyword_rules.whatsapp_account = wa.name AND keyword_rules.organization_id = wa.organization_id;
ALTER TABLE keyword_rules DROP COLUMN whatsapp_account;
CREATE INDEX idx_keyword_rules_whatsapp_account_id ON keyword_rules(whatsapp_account_id);

-- Update chatbot_flows
ALTER TABLE chatbot_flows ADD COLUMN whatsapp_account_id UUID;
UPDATE chatbot_flows SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE chatbot_flows.whatsapp_account = wa.name AND chatbot_flows.organization_id = wa.organization_id;
ALTER TABLE chatbot_flows DROP COLUMN whatsapp_account;
CREATE INDEX idx_chatbot_flows_whatsapp_account_id ON chatbot_flows(whatsapp_account_id);

-- Update chatbot_sessions
ALTER TABLE chatbot_sessions ADD COLUMN whatsapp_account_id UUID;
UPDATE chatbot_sessions SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE chatbot_sessions.whatsapp_account = wa.name AND chatbot_sessions.organization_id = wa.organization_id;
ALTER TABLE chatbot_sessions DROP COLUMN whatsapp_account;
CREATE INDEX idx_chatbot_sessions_whatsapp_account_id ON chatbot_sessions(whatsapp_account_id);

-- Update ai_contexts
ALTER TABLE ai_contexts ADD COLUMN whatsapp_account_id UUID;
UPDATE ai_contexts SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE ai_contexts.whatsapp_account = wa.name AND ai_contexts.organization_id = wa.organization_id;
ALTER TABLE ai_contexts DROP COLUMN whatsapp_account;
CREATE INDEX idx_ai_contexts_whatsapp_account_id ON ai_contexts(whatsapp_account_id);

-- Update agent_transfers
ALTER TABLE agent_transfers ADD COLUMN whatsapp_account_id UUID;
UPDATE agent_transfers SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE agent_transfers.whatsapp_account = wa.name AND agent_transfers.organization_id = wa.organization_id;
ALTER TABLE agent_transfers DROP COLUMN whatsapp_account;
CREATE INDEX idx_agent_transfers_whatsapp_account_id ON agent_transfers(whatsapp_account_id);

-- Update bulk_message_campaigns
ALTER TABLE bulk_message_campaigns ADD COLUMN whatsapp_account_id UUID;
UPDATE bulk_message_campaigns SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE bulk_message_campaigns.whatsapp_account = wa.name AND bulk_message_campaigns.organization_id = wa.organization_id;
ALTER TABLE bulk_message_campaigns DROP COLUMN whatsapp_account;
CREATE INDEX idx_bulk_message_campaigns_whatsapp_account_id ON bulk_message_campaigns(whatsapp_account_id);

-- Update call_logs
ALTER TABLE call_logs ADD COLUMN whatsapp_account_id UUID;
UPDATE call_logs SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE call_logs.whatsapp_account = wa.name AND call_logs.organization_id = wa.organization_id;
ALTER TABLE call_logs DROP COLUMN whatsapp_account;
CREATE INDEX idx_call_logs_whatsapp_account_id ON call_logs(whatsapp_account_id);

-- Update call_transfers
ALTER TABLE call_transfers ADD COLUMN whatsapp_account_id UUID;
UPDATE call_transfers SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE call_transfers.whatsapp_account = wa.name AND call_transfers.organization_id = wa.organization_id;
ALTER TABLE call_transfers DROP COLUMN whatsapp_account;
CREATE INDEX idx_call_transfers_whatsapp_account_id ON call_transfers(whatsapp_account_id);

-- Update catalogs
ALTER TABLE catalogs ADD COLUMN whatsapp_account_id UUID;
UPDATE catalogs SET whatsapp_account_id = wa.id FROM whatsapp_accounts wa WHERE catalogs.whatsapp_account = wa.name AND catalogs.organization_id = wa.organization_id;
ALTER TABLE catalogs DROP COLUMN whatsapp_account;
CREATE INDEX idx_catalogs_whatsapp_account_id ON catalogs(whatsapp_account_id);

