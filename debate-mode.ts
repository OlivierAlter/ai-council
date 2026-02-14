/**
 * AI Council Debate Mode Implementation
 * Multi-agent iterative review and consensus building
 */

import { SessionManager, SessionState } from './websocket-server'

// Debate Mode Types
type DebateAgent = {
  id: string
  name: string
  role: string
  expertise: string[]
  currentTask?: string
}

type DebateTurn = {
  agentId: string
  turnNumber: number
  action: 'analysis' | 'review' | 'refinement' | 'consensus_check'
  content: string
  timestamp: Date
  toolUses?: ToolUse[]
}

type ToolUse = {
  toolName: string
  input: any
  output?: any
  status: 'pending' | 'approved' | 'denied' | 'completed' | 'failed'
}

type DebateState = {
  debateId: string
  sessionId: string
  agents: DebateAgent[]
  currentTurn: number
  turns: DebateTurn[]
  initialPrompt: string
  currentFocus: string
  consensusReached: boolean
  consensusSummary?: string
  createdAt: Date
  updatedAt: Date
}

// Enhanced Debate Mode Coordinator
export class EnhancedDebateModeCoordinator {
  private activeDebates: Map<string, DebateState>
  private sessionManager: SessionManager
  
  constructor(sessionManager: SessionManager) {
    this.activeDebates = new Map()
    this.sessionManager = sessionManager
  }
  
  async startDebate(
    sessionId: string,
    agents: DebateAgent[],
    initialPrompt: string,
    debateId?: string
  ): Promise<DebateState> {
    const session = this.sessionManager.getSession(sessionId)
    if (!session) {
      throw new Error('Session not found')
    }
    
    const debateIdToUse = debateId || `debate_${Date.now()}`
    
    const debateState: DebateState = {
      debateId: debateIdToUse,
      sessionId,
      agents,
      currentTurn: 0,
      turns: [],
      initialPrompt,
      currentFocus: initialPrompt,
      consensusReached: false,
      createdAt: new Date(),
      updatedAt: new Date()
    }
    
    this.activeDebates.set(debateIdToUse, debateState)
    
    // Start the debate cycle
    await this.runDebateCycle(debateState)
    
    return debateState
  }
  
  private async runDebateCycle(debateState: DebateState): Promise<void> {
    const maxTurns = 10 // Safety limit
    let turnNumber = 0
    
    while (turnNumber < maxTurns && !debateState.consensusReached) {
      turnNumber++
      debateState.currentTurn = turnNumber
      
      // Each agent gets a turn
      for (const agent of debateState.agents) {
        if (debateState.consensusReached) break
        
        await this.runAgentTurn(debateState, agent, turnNumber)
        
        // Check for consensus after each agent's turn
        if (turnNumber > 1) { // Don't check on first turn
          const consensus = await this.checkConsensus(debateState)
          if (consensus) {
            debateState.consensusReached = true
            debateState.consensusSummary = consensus
            break
          }
        }
      }
      
      debateState.updatedAt = new Date()
      await new Promise(resolve => setTimeout(resolve, 1000)) // Small delay between turns
    }
    
    if (!debateState.consensusReached) {
      console.log(`Debate ${debateState.debateId} reached maximum turns without consensus`)
    } else {
      console.log(`Debate ${debateState.debateId} reached consensus: ${debateState.consensusSummary}`)
    }
  }
  
  private async runAgentTurn(debateState: DebateState, agent: DebateAgent, turnNumber: number): Promise<void> {
    // Determine the agent's action based on turn number and role
    let action: DebateTurn['action']
    
    if (turnNumber === 1) {
      action = 'analysis' // First turn: initial analysis
    } else if (turnNumber % debateState.agents.length === 0) {
      action = 'consensus_check' // Every full cycle: check consensus
    } else if (turnNumber % 2 === 0) {
      action = 'review' // Even turns: review other agents' work
    } else {
      action = 'refinement' // Odd turns: refine own analysis
    }
    
    // Create the turn
    const turn: DebateTurn = {
      agentId: agent.id,
      turnNumber,
      action,
      content: '',
      timestamp: new Date(),
      toolUses: []
    }
    
    // Generate the agent's task based on current debate state
    const task = this.generateAgentTask(debateState, agent, action)
    agent.currentTask = task
    
    // Send the task to the agent (via WebSocket user message)
    const userMessage = this.createUserMessageForAgent(debateState.sessionId, agent, task)
    
    // In a real implementation, this would send to the WebSocket
    console.log(`[DEBATE ${debateState.debateId}] Agent ${agent.name} (${agent.role}): ${task.substring(0, 100)}...`)
    
    // Simulate agent processing (in real implementation, wait for assistant response)
    const simulatedResponse = await this.simulateAgentResponse(agent, task, debateState)
    turn.content = simulatedResponse
    
    // Add any tool uses from the response
    if (simulatedResponse.includes('tool_use')) {
      turn.toolUses = [
        {
          toolName: 'AnalysisTool',
          input: { query: task },
          status: 'completed'
        }
      ]
    }
    
    debateState.turns.push(turn)
    
    // Update the current focus based on this turn
    if (action === 'analysis' || action === 'refinement') {
      debateState.currentFocus = `Analysis from ${agent.name}: ${simulatedResponse.substring(0, 50)}...`
    }
  }
  
  private generateAgentTask(debateState: DebateState, agent: DebateAgent, action: DebateTurn['action']): string {
    const previousTurns = debateState.turns
    const otherAgents = debateState.agents.filter(a => a.id !== agent.id)
    
    switch (action) {
      case 'analysis':
        return `You are ${agent.name}, a ${agent.role} with expertise in ${agent.expertise.join(', ')}.
        
Initial Task: ${debateState.initialPrompt}
        
Please provide a comprehensive analysis of this problem from your perspective. Be specific about:
        1. Key issues and challenges
        2. Potential solutions or approaches
        3. Risks and considerations
        4. Any domain-specific insights
        
Your analysis will be reviewed by other experts, so be thorough and evidence-based.`
      
      case 'review':
        const reviewsNeeded = previousTurns.filter(t => t.agentId !== agent.id && t.action === 'analysis')
        if (reviewsNeeded.length === 0) return this.generateAgentTask(debateState, agent, 'refinement')
        
        return `You are ${agent.name}, reviewing analyses from other experts.
        
Previous Analyses:
        ${reviewsNeeded.map((t, i) => `${i+1}. ${debateState.agents.find(a => a.id === t.agentId)?.name} (${debateState.agents.find(a => a.id === t.agentId)?.role}): ${t.content.substring(0, 200)}...`).join('\n        ')}
        
Your Review Task:
        1. Evaluate the strengths and weaknesses of each analysis
        2. Identify any gaps, errors, or oversights
        3. Suggest improvements or alternative perspectives
        4. Highlight areas of agreement and disagreement
        
Be constructive and specific in your feedback.`
      
      case 'refinement':
        const agentPreviousTurns = previousTurns.filter(t => t.agentId === agent.id)
        const feedbackForAgent = previousTurns.filter(t => t.action === 'review' && t.content.includes(agent.name))
        
        return `You are ${agent.name}, refining your previous analysis based on feedback.
        
Your Previous Analysis:
        ${agentPreviousTurns.length > 0 ? agentPreviousTurns[agentPreviousTurns.length - 1].content : 'None yet'}
        
Feedback Received:
        ${feedbackForAgent.length > 0 ? feedbackForAgent.map(f => `- ${f.content.substring(0, 150)}...`).join('\n        ') : 'No specific feedback yet'}
        
Refinement Task:
        1. Address the feedback and improve your analysis
        2. Incorporate valid criticisms and suggestions
        3. Provide additional evidence or reasoning where needed
        4. Maintain your unique perspective while being open to other viewpoints
        
Create an improved version of your analysis.`
      
      case 'consensus_check':
        return `You are ${agent.name}, evaluating whether consensus has been reached.
        
Current State of Debate:
        - Initial Prompt: ${debateState.initialPrompt}
        - Turns Completed: ${debateState.currentTurn}
        - Current Focus: ${debateState.currentFocus}
        
All Analyses and Reviews:
        ${previousTurns.map((t, i) => `${i+1}. ${debateState.agents.find(a => a.id === t.agentId)?.name} (${t.action}): ${t.content.substring(0, 100)}...`).join('\n        ')}
        
Consensus Evaluation Task:
        1. Assess whether the group has reached a satisfactory solution
        2. Identify any remaining major disagreements or unresolved issues
        3. Determine if further iteration would be productive
        4. If consensus is reached, summarize the key conclusions
        5. If not, suggest what specific areas need more work
        
Be honest in your assessment - it's okay if consensus hasn't been reached yet.`
    }
  }
  
  private createUserMessageForAgent(sessionId: string, agent: DebateAgent, task: string): any {
    return {
      type: 'user',
      message: {
        role: 'user',
        content: task
      },
      parent_tool_use_id: null,
      session_id: sessionId,
      uuid: crypto.randomUUID()
    }
  }
  
  private async simulateAgentResponse(agent: DebateAgent, task: string, debateState: DebateState): Promise<string> {
    // In a real implementation, this would wait for the actual assistant response
    // For simulation, generate a plausible response
    
    await new Promise(resolve => setTimeout(resolve, 500)) // Simulate thinking time
    
    if (task.includes('Consensus Evaluation')) {
      // Simulate consensus check
      const shouldReachConsensus = debateState.turns.length > 6 || Math.random() > 0.7
      if (shouldReachConsensus) {
        return `After reviewing all the analyses and discussions, I believe we have reached a satisfactory consensus. 
        
Key Conclusions:
        1. The main issue is ${debateState.initialPrompt.split(' ').slice(0, 5).join(' ')}...
        2. The best approach involves ${Math.random() > 0.5 ? 'a combination of techniques' : 'a focused solution'}
        3. We should prioritize ${['safety', 'efficiency', 'comprehensiveness', 'innovation'][Math.floor(Math.random() * 4)]}
        
All major concerns have been addressed through the iterative review process.`
      } else {
        return `After evaluating the current state, I don't believe we have reached consensus yet. 
        
Remaining Issues:
        1. We still have differing opinions on ${['the root cause', 'the best approach', 'the implementation details', 'the success criteria'][Math.floor(Math.random() * 4)]}
        2. ${agent.name} and ${debateState.agents.find(a => a.id !== agent.id)?.name} have conflicting analyses that need resolution
        3. More evidence is needed to support some of the key claims
        
I recommend at least one more iteration of analysis and review.`
      }
    } else if (task.includes('reviewing analyses')) {
      return `${agent.name}'s Review:
      
Strengths observed:
      - Comprehensive coverage of ${['technical aspects', 'business implications', 'user experience', 'security considerations'][Math.floor(Math.random() * 4)]}
      - Good use of ${['data', 'examples', 'analogies', 'industry standards'][Math.floor(Math.random() * 4)]}
      
Areas for improvement:
      - Could benefit from more ${['quantitative analysis', 'specific examples', 'risk assessment', 'alternative perspectives'][Math.floor(Math.random() * 4)]}
      - The section on ${['implementation', 'costs', 'timeline', 'dependencies'][Math.floor(Math.random() * 4)]} needs more detail
      - Should consider ${['edge cases', 'long-term maintenance', 'user adoption', 'regulatory compliance'][Math.floor(Math.random() * 4)]}
      
Overall, this is a solid analysis that could be strengthened with these additions.`
    } else {
      return `${agent.name}'s ${task.includes('refining') ? 'Refined ' : ''}Analysis:
      
Problem Summary: ${debateState.initialPrompt.substring(0, 50)}...
      
Key Insights from ${agent.role} Perspective:
      1. ${['The core issue appears to be', 'Our analysis reveals that', 'Initial investigation shows'][Math.floor(Math.random() * 3)]} ${debateState.initialPrompt.split(' ').slice(0, 8).join(' ')}...
      2. ${['This suggests we should', 'The data indicates that', 'Our expertise points to'][Math.floor(Math.random() * 3)]} ${['a multi-phase approach', 'focusing on key leverage points', 'prioritizing quick wins', 'developing a comprehensive strategy'][Math.floor(Math.random() * 4)]}
      3. ${['Potential risks include', 'We must be cautious about', 'Important considerations are'][Math.floor(Math.random() * 3)]} ${['resource constraints', 'technical debt', 'user resistance', 'unintended consequences'][Math.floor(Math.random() * 4)]}
      
Recommended Actions:
      - ${['Conduct further analysis on', 'Implement pilot testing for', 'Develop prototypes to validate', 'Create detailed specifications for'][Math.floor(Math.random() * 4)]} ${debateState.initialPrompt.split(' ').slice(0, 4).join(' ')}...
      - ${['Establish clear metrics for', 'Define success criteria around', 'Create feedback loops to measure', 'Build monitoring for'][Math.floor(Math.random() * 4)]} progress
      - ${['Engage stakeholders to', 'Coordinate with teams on', 'Align with organizational goals for', 'Ensure compatibility with'][Math.floor(Math.random() * 4)]} implementation
      
This analysis is based on ${agent.expertise.join(', ')} expertise and ${['industry best practices', 'empirical data', 'proven methodologies', 'extensive experience'][Math.floor(Math.random() * 4)]}.`
    }
  }
  
  private async checkConsensus(debateState: DebateState): Promise<string | null> {
    // Look for consensus check turns
    const consensusTurns = debateState.turns.filter(t => t.action === 'consensus_check')
    
    if (consensusTurns.length === 0) return null
    
    // Simple consensus detection: if majority of consensus checks say consensus is reached
    const positiveConsensus = consensusTurns.filter(t => 
      t.content.includes('consensus') && t.content.includes('reached')
    )
    
    if (positiveConsensus.length >= Math.ceil(consensusTurns.length * 0.66)) {
      // Extract key points from the consensus statements
      const keyPoints = positiveConsensus.map(t => {
        const match = t.content.match(/Key Conclusions:\s*([\s\S]*)/i)
        return match ? match[1].split('\n').filter(line => line.trim().startsWith('1.') || line.trim().startsWith('2.') || line.trim().startsWith('3.')).join(' ') : ''
      }).filter(p => p).join(' ')
      
      return `Consensus reached after ${debateState.currentTurn} turns. ${keyPoints.substring(0, 200)}...`
    }
    
    return null
  }
  
  getDebateState(debateId: string): DebateState | undefined {
    return this.activeDebates.get(debateId)
  }
  
  listActiveDebates(): DebateState[] {
    return Array.from(this.activeDebates.values())
  }
  
  async endDebate(debateId: string, finalSummary: string): Promise<void> {
    const debate = this.activeDebates.get(debateId)
    if (debate) {
      debate.consensusReached = true
      debate.consensusSummary = finalSummary
      debate.updatedAt = new Date()
      console.log(`Debate ${debateId} ended with summary: ${finalSummary.substring(0, 100)}...`)
    }
  }
}

// Real-time TUI Streaming Integration
export class TUIStreamingBridge {
  private sessionManager: SessionManager
  private debateCoordinator: EnhancedDebateModeCoordinator
  private activeStreams: Map<string, TUIStreamState>
  
  constructor(sessionManager: SessionManager, debateCoordinator: EnhancedDebateModeCoordinator) {
    this.sessionManager = sessionManager
    this.debateCoordinator = debateCoordinator
    this.activeStreams = new Map()
  }
  
  startStreaming(sessionId: string, outputCallback: (data: string) => void): string {
    const streamId = `stream_${Date.now()}`
    
    const streamState: TUIStreamState = {
      streamId,
      sessionId,
      outputCallback,
      isActive: true,
      createdAt: new Date(),
      lastUpdate: new Date()
    }
    
    this.activeStreams.set(streamId, streamState)
    
    // Set up message forwarding
    this.setupMessageForwarding(streamState)
    
    return streamId
  }
  
  private setupMessageForwarding(streamState: TUIStreamState): void {
    // In a real implementation, this would hook into the WebSocket message handler
    // to forward relevant messages to the TUI
    console.log(`TUI streaming started for session ${streamState.sessionId}`)
  }
  
  handleStreamEvent(sessionId: string, event: any): void {
    // Forward stream events to all active TUI streams for this session
    const streams = Array.from(this.activeStreams.values())
      .filter(s => s.sessionId === sessionId && s.isActive)
    
    for (const stream of streams) {
      try {
        if (event.event && event.event.type === 'content_block_delta') {
          const text = event.event.delta?.text
          if (text) {
            stream.outputCallback(text)
          }
        }
        
        // Update stream state
        stream.lastUpdate = new Date()
      } catch (error) {
        console.error('Error in TUI streaming:', error)
      }
    }
  }
  
  handleDebateUpdate(debateState: DebateState): void {
    // Forward debate updates to TUI
    const streams = Array.from(this.activeStreams.values())
      .filter(s => s.sessionId === debateState.sessionId && s.isActive)
    
    for (const stream of streams) {
      try {
        const update = this.formatDebateUpdate(debateState)
        stream.outputCallback(`\n[DEBATE UPDATE] ${update}\n`)
        stream.lastUpdate = new Date()
      } catch (error) {
        console.error('Error in debate streaming:', error)
      }
    }
  }
  
  private formatDebateUpdate(debateState: DebateState): string {
    const lastTurn = debateState.turns[debateState.turns.length - 1]
    const agent = debateState.agents.find(a => a.id === lastTurn?.agentId)
    
    if (debateState.consensusReached) {
      return `🎉 Consensus reached! ${debateState.consensusSummary}`
    } else if (lastTurn) {
      return `Turn ${lastTurn.turnNumber}: ${agent?.name} (${lastTurn.action}) - ${lastTurn.content.substring(0, 80)}...`
    } else {
      return `Debate started: ${debateState.initialPrompt.substring(0, 60)}...`
    }
  }
  
  stopStreaming(streamId: string): void {
    const stream = this.activeStreams.get(streamId)
    if (stream) {
      stream.isActive = false
      this.activeStreams.delete(streamId)
      console.log(`TUI streaming stopped for ${stream.sessionId}`)
    }
  }
  
  cleanupInactiveStreams(): void {
    const now = new Date()
    const inactiveThreshold = 30 * 60 * 1000 // 30 minutes
    
    for (const [streamId, stream] of this.activeStreams) {
      if (!stream.isActive || (now.getTime() - stream.lastUpdate.getTime()) > inactiveThreshold) {
        this.activeStreams.delete(streamId)
        console.log(`Cleaned up inactive stream ${streamId}`)
      }
    }
  }
}

interface TUIStreamState {
  streamId: string
  sessionId: string
  outputCallback: (data: string) => void
  isActive: boolean
  createdAt: Date
  lastUpdate: Date
}

// Export types for use in other modules
export type {
  DebateAgent,
  DebateTurn,
  ToolUse,
  DebateState,
  TUIStreamState
}
