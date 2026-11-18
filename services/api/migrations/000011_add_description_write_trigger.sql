-- Extend the agent_conversations trigger_type constraint to include
-- 'description_write'.
--
-- This file also used to add agents.description_write_trigger_prompt. That
-- column is retired by 000019_drop_agent_trigger_prompts.sql, and re-adding it
-- on every boot burnt one attnum per boot until agents hit the 1600-column
-- limit (SPA-8755) — see 000010.

-- Extend the trigger_type check constraint to include 'description_write'.
ALTER TABLE agent_conversations
    DROP CONSTRAINT IF EXISTS agent_conversations_trigger_type_check;

ALTER TABLE agent_conversations
    ADD CONSTRAINT agent_conversations_trigger_type_check
    CHECK (trigger_type IN ('task_assigned', 'comment_mention', 'chat_message', 'description_write'));
