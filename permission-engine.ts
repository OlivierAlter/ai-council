/**
 * Advanced Permission Engine for AI Council
 * Programmatic tool permission handling with policy-based decisions
 */

import { SessionManager, SessionState } from './websocket-server'
import { ControlResponseMessage } from './websocket-server'

// Permission Types and Interfaces
type PermissionRule = {
  id: string
  toolName: string
  pattern?: string // Glob pattern or regex for input matching
  behavior: 'allow' | 'deny' | 'ask'
  scope: 'global' | 'session' | 'project'
  createdAt: Date
  createdBy?: string
  reason?: string
}

type PermissionPolicy = {
  id: string
  name: string
  description: string
  rules: PermissionRule[]
  defaultBehavior: 'allow' | 'deny' | 'ask'
  appliesTo: 'all' | 'specific-tools'
  specificTools?: string[]
}

type ToolPermissionRequest = {
  requestId: string
  sessionId: string
  toolName: string
  input: Record<string, unknown>
  toolUseId: string
  agentId?: string
  context: {
    currentTask?: string
    debateContext?: any
    sessionState: SessionState
  }
}

type PermissionDecision = {
  behavior: 'allow' | 'deny'
  updatedInput: Record<string, unknown>
  updatedPermissions?: PermissionUpdate[]
  message?: string
  interrupt?: boolean
}

type PermissionUpdate = {
  type: 'addRules' | 'replaceRules' | 'removeRules' | 'setMode'
  rules?: PermissionRule[]
  behavior?: 'allow' | 'deny' | 'ask'
  destination: 'userSettings' | 'projectSettings' | 'localSettings' | 'session' | 'cliArg'
}

// Advanced Permission Engine
export class AdvancedPermissionEngine {
  private sessionManager: SessionManager
  private policies: PermissionPolicy[]
  private globalRules: PermissionRule[]
  private auditLog: PermissionAuditEntry[]
  
  constructor(sessionManager: SessionManager) {
    this.sessionManager = sessionManager
    this.policies = []
    this.globalRules = []
    this.auditLog = []
    
    // Initialize with some default policies
    this.initializeDefaultPolicies()
  }
  
  private initializeDefaultPolicies(): void {
    // Default security policy
    this.addPolicy({
      id: 'default-security',
      name: 'Default Security Policy',
      description: 'Basic security rules for all sessions',
      rules: [
        {
          id: 'deny-rm-rf',
          toolName: 'Bash',
          pattern: '*rm -rf*',
          behavior: 'deny',
          scope: 'global',
          reason: 'Prevent accidental recursive deletion'
        },
        {
          id: 'deny-chmod-recursive',
          toolName: 'Bash',
          pattern: '*chmod -R*',
          behavior: 'deny',
          scope: 'global',
          reason: 'Prevent accidental permission changes'
        }
      ],
      defaultBehavior: 'ask',
      appliesTo: 'all'
    })
    
    // Read-only policy for analysis tools
    this.addPolicy({
      id: 'read-only-analysis',
      name: 'Read-Only Analysis Policy',
      description: 'Allow read operations but deny write operations for analysis tools',
      rules: [
        {
          id: 'allow-read-operations',
          toolName: 'Read',
          behavior: 'allow',
          scope: 'global'
        },
        {
          id: 'allow-glob-operations',
          toolName: 'Glob',
          behavior: 'allow',
          scope: 'global'
        },
        {
          id: 'allow-grep-operations',
          toolName: 'Grep',
          behavior: 'allow',
          scope: 'global'
        }
      ],
      defaultBehavior: 'ask',
      appliesTo: 'specific-tools',
      specificTools: ['Read', 'Glob', 'Grep', 'Bash']
    })
  }
  
  addPolicy(policy: PermissionPolicy): void {
    this.policies.push(policy)
    console.log(`Added permission policy: ${policy.name}`)
  }
  
  addGlobalRule(rule: PermissionRule): void {
    this.globalRules.push(rule)
    console.log(`Added global permission rule: ${rule.id}`)
  }
  
  async handlePermissionRequest(
    sessionId: string,
    requestId: string,
    controlRequest: any
  ): Promise<ControlResponseMessage> {
    const session = this.sessionManager.getSession(sessionId)
    if (!session) {
      return this.createErrorResponse(requestId, 'Session not found')
    }
    
    // Parse the permission request
    const permissionRequest: ToolPermissionRequest = {
      requestId,
      sessionId,
      toolName: controlRequest.request.tool_name,
      input: controlRequest.request.input || {},
      toolUseId: controlRequest.request.tool_use_id,
      agentId: controlRequest.request.agent_id,
      context: {
        sessionState: session
      }
    }
    
    // Log the request
    this.logAuditEntry({
      type: 'request',
      requestId,
      sessionId,
      toolName: permissionRequest.toolName,
      input: permissionRequest.input,
      timestamp: new Date()
    })
    
    try {
      // Evaluate the permission request against policies
      const decision = await this.evaluatePermissionRequest(permissionRequest)
      
      // Create the response
      const response = this.createPermissionResponse(decision)
      
      // Log the decision
      this.logAuditEntry({
        type: 'decision',
        requestId,
        sessionId,
        toolName: permissionRequest.toolName,
        decision: decision.behavior,
        message: decision.message,
        timestamp: new Date()
      })
      
      return this.createSuccessResponse(requestId, response)
    } catch (error) {
      console.error('Permission evaluation error:', error)
      return this.createErrorResponse(requestId, error instanceof Error ? error.message : 'Unknown error')
    }
  }
  
  private async evaluatePermissionRequest(request: ToolPermissionRequest): Promise<PermissionDecision> {
    const { toolName, input, sessionId } = request
    const session = this.sessionManager.getSession(sessionId)
    
    if (!session) {
      throw new Error('Session not found')
    }
    
    // Check global rules first (highest priority)
    const globalRuleDecision = this.checkRulesAgainstInput(this.globalRules, toolName, input)
    if (globalRuleDecision) {
      return this.createDecisionFromRule(globalRuleDecision, request, 'Global rule match')
    }
    
    // Check applicable policies
    const applicablePolicies = this.getApplicablePolicies(toolName)
    
    for (const policy of applicablePolicies) {
      const policyDecision = this.checkRulesAgainstInput(policy.rules, toolName, input)
      if (policyDecision) {
        return this.createDecisionFromRule(policyDecision, request, `Policy: ${policy.name}`)
      }
    }
    
    // Check session-specific rules
    if (session.permissionRules) {
      const sessionRuleDecision = this.checkRulesAgainstInput(session.permissionRules, toolName, input)
      if (sessionRuleDecision) {
        return this.createDecisionFromRule(sessionRuleDecision, request, 'Session rule match')
      }
    }
    
    // Fall back to default behavior based on permission mode
    return this.getDefaultDecision(request, session.permissionMode)
  }
  
  private getApplicablePolicies(toolName: string): PermissionPolicy[] {
    return this.policies.filter(policy => {
      if (policy.appliesTo === 'all') return true
      if (policy.appliesTo === 'specific-tools') {
        return policy.specificTools?.includes(toolName) || false
      }
      return false
    })
  }
  
  private checkRulesAgainstInput(
    rules: PermissionRule[],
    toolName: string,
    input: Record<string, unknown>
  ): PermissionRule | null {
    for (const rule of rules) {
      if (rule.toolName !== toolName) continue
      
      // Check if input matches the pattern (if specified)
      if (rule.pattern) {
        const inputString = this.stringifyInput(input)
        if (this.matchesPattern(inputString, rule.pattern)) {
          return rule
        }
      } else {
        // No pattern - rule applies to all uses of this tool
        return rule
      }
    }
    
    return null
  }
  
  private stringifyInput(input: Record<string, unknown>): string {
    try {
      return JSON.stringify(input, null, 2)
    } catch {
      return String(input)
    }
  }
  
  private matchesPattern(input: string, pattern: string): boolean {
    // Simple pattern matching - could be enhanced with proper glob/regex
    if (pattern.startsWith('*') && pattern.endsWith('*')) {
      const searchTerm = pattern.slice(1, -1).toLowerCase()
      return input.toLowerCase().includes(searchTerm)
    } else if (pattern.startsWith('*')) {
      const searchTerm = pattern.slice(1).toLowerCase()
      return input.toLowerCase().endsWith(searchTerm)
    } else if (pattern.endsWith('*')) {
      const searchTerm = pattern.slice(0, -1).toLowerCase()
      return input.toLowerCase().startsWith(searchTerm)
    } else {
      return input.includes(pattern)
    }
  }
  
  private createDecisionFromRule(
    rule: PermissionRule,
    request: ToolPermissionRequest,
    reason: string
  ): PermissionDecision {
    const { behavior, toolName } = rule
    
    switch (behavior) {
      case 'allow':
        return {
          behavior: 'allow',
          updatedInput: request.input,
          updatedPermissions: this.createLearningRules(rule, request),
          message: `Allowed by ${reason}: ${rule.reason || 'No specific reason'}`
        }
      
      case 'deny':
        return {
          behavior: 'deny',
          updatedInput: this.sanitizeDeniedInput(toolName, request.input),
          message: `Denied by ${reason}: ${rule.reason || 'Security policy violation'}`,
          interrupt: rule.id === 'deny-rm-rf' // Interrupt session for critical violations
        }
      
      case 'ask':
        // In ask mode, we still need to return a decision
        // For now, we'll ask by denying with a specific message
        return {
          behavior: 'deny',
          updatedInput: request.input,
          message: `Manual approval required by ${reason}: ${rule.reason || 'Policy requires review'}`
        }
    }
  }
  
  private sanitizeDeniedInput(toolName: string, input: Record<string, unknown>): Record<string, unknown> {
    // Modify dangerous inputs to be safe
    if (toolName === 'Bash') {
      const command = input.command
      if (typeof command === 'string' && command.includes('rm -rf')) {
        return { ...input, command: 'echo "Command blocked by security policy"' }
      }
    }
    
    return input
  }
  
  private createLearningRules(rule: PermissionRule, request: ToolPermissionRequest): PermissionUpdate[] {
    // Create rules that can be learned from this decision
    if (rule.scope === 'session' || rule.behavior === 'allow') {
      return [
        {
          type: 'addRules',
          rules: [{
            toolName: request.toolName,
            pattern: this.stringifyInput(request.input),
            behavior: rule.behavior,
            scope: 'session'
          }],
          destination: 'session'
        }
      ]
    }
    
    return []
  }
  
  private getDefaultDecision(request: ToolPermissionRequest, permissionMode: string): PermissionDecision {
    // Base decision on the session's permission mode
    switch (permissionMode) {
      case 'bypassPermissions':
        return {
          behavior: 'allow',
          updatedInput: request.input,
          message: 'Allowed by bypassPermissions mode'
        }
      
      case 'dontAsk':
        return {
          behavior: 'deny',
          updatedInput: request.input,
          message: 'Denied by dontAsk mode'
        }
      
      case 'acceptEdits':
        if (['Edit', 'Write'].includes(request.toolName)) {
          return {
            behavior: 'allow',
            updatedInput: request.input,
            message: 'Allowed by acceptEdits mode for edit operations'
          }
        }
        // Fall through to default for non-edit tools
      
      default: // 'default', 'plan', 'delegate'
        return {
          behavior: 'deny',
          updatedInput: request.input,
          message: 'Manual approval required by default permission mode'
        }
    }
  }
  
  private createPermissionResponse(decision: PermissionDecision): any {
    return {
      behavior: decision.behavior,
      updatedInput: decision.updatedInput,
      updatedPermissions: decision.updatedPermissions,
      toolUseID: decision.behavior === 'allow' ? crypto.randomUUID() : undefined
    }
  }
  
  private createSuccessResponse(requestId: string, response: any): ControlResponseMessage {
    return {
      type: 'control_response',
      session_id: '', // Will be filled by caller
      response: {
        subtype: 'success',
        request_id: requestId,
        response
      }
    }
  }
  
  private createErrorResponse(requestId: string, error: string): ControlResponseMessage {
    return {
      type: 'control_response',
      session_id: '', // Will be filled by caller
      response: {
        subtype: 'error',
        request_id: requestId,
        error
      }
    }
  }
  
  private logAuditEntry(entry: PermissionAuditEntry): void {
    this.auditLog.push(entry)
    // In production, this would also write to persistent storage
    console.log(`[AUDIT] ${entry.type}: ${entry.toolName} - ${entry.requestId}`)
  }
  
  getAuditLog(sessionId?: string): PermissionAuditEntry[] {
    if (sessionId) {
      return this.auditLog.filter(entry => entry.sessionId === sessionId)
    }
    return [...this.auditLog]
  }
  
  getPermissionStatistics(): PermissionStatistics {
    const totalRequests = this.auditLog.filter(entry => entry.type === 'request').length
    const totalDecisions = this.auditLog.filter(entry => entry.type === 'decision').length
    
    const allowed = this.auditLog.filter(entry => 
      entry.type === 'decision' && entry.decision === 'allow'
    ).length
    
    const denied = this.auditLog.filter(entry => 
      entry.type === 'decision' && entry.decision === 'deny'
    ).length
    
    return {
      totalRequests,
      totalDecisions,
      allowed,
      denied,
      allowRate: totalDecisions > 0 ? (allowed / totalDecisions) * 100 : 0,
      denyRate: totalDecisions > 0 ? (denied / totalDecisions) * 100 : 0
    }
  }
  
  // Debate Mode Integration
  async handleDebatePermissionRequest(
    request: ToolPermissionRequest,
    debateContext: any
  ): Promise<PermissionDecision> {
    // Enhance the request with debate context
    request.context.debateContext = debateContext
    
    // Special handling for debate mode
    if (debateContext?.phase === 'consensus_check') {
      // During consensus checking, be more permissive with analysis tools
      if (['Read', 'Glob', 'Grep', 'Bash'].includes(request.toolName)) {
        const inputStr = this.stringifyInput(request.input)
        if (!inputStr.includes('rm ') && !inputStr.includes('chmod ')) {
          return {
            behavior: 'allow',
            updatedInput: request.input,
            message: 'Allowed during consensus checking phase for analysis tools'
          }
        }
      }
    }
    
    // Otherwise use normal evaluation
    return this.evaluatePermissionRequest(request)
  }
}

interface PermissionAuditEntry {
  type: 'request' | 'decision'
  requestId: string
  sessionId: string
  toolName: string
  input?: any
  decision?: 'allow' | 'deny'
  message?: string
  timestamp: Date
}

interface PermissionStatistics {
  totalRequests: number
  totalDecisions: number
  allowed: number
  denied: number
  allowRate: number
  denyRate: number
}

// Extend SessionState to include permission rules
declare module './websocket-server' {
  interface SessionState {
    permissionRules?: PermissionRule[]
  }
}

export type {
  PermissionRule,
  PermissionPolicy,
  ToolPermissionRequest,
  PermissionDecision,
  PermissionUpdate,
  PermissionAuditEntry,
  PermissionStatistics
}
