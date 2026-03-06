-- Agent audit log for tracking commissioner actions via AI agent
CREATE TABLE IF NOT EXISTS agent_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    tool_name TEXT NOT NULL,
    tool_args JSONB NOT NULL DEFAULT '{}',
    tool_result JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_audit_log_user_id ON agent_audit_log(user_id);
CREATE INDEX idx_agent_audit_log_created_at ON agent_audit_log(created_at);
