const WebSocket = require('ws');
const { v4: uuidv4 } = require('uuid');

const PORT = process.env.PORT || 3000;
const wss = new WebSocket.Server({ port: PORT });

console.log(`AI Council SDK Server running on ws://localhost:${PORT}`);

// State management
const agents = new Map(); // sessionId -> { socket, type: 'agent', capabilities: {}, status: 'idle' }
const clients = new Map(); // connectionId -> { socket, type: 'client' }
const activeDebates = new Map(); // debateId -> { topic, proAgentId, conAgentId, turn: 0, history: [] }

wss.on('connection', (ws, req) => {
    const connectionId = uuidv4();
    const url = new URL(req.url, `http://${req.headers.host}`);
    const role = url.searchParams.get('role') || 'agent'; // Default to agent if not specified
    
    console.log(`New connection: ${connectionId} (Role: ${role})`);

    if (role === 'client') {
        handleClientConnection(ws, connectionId);
    } else {
        handleAgentConnection(ws, connectionId, req);
    }
});

function handleClientConnection(ws, connectionId) {
    clients.set(connectionId, { socket: ws, type: 'client' });

    ws.on('message', (message) => {
        try {
            const data = JSON.parse(message);
            handleClientMessage(connectionId, data);
        } catch (e) {
            console.error('Error parsing client message:', e);
        }
    });

    ws.on('close', () => {
        console.log(`Client disconnected: ${connectionId}`);
        clients.delete(connectionId);
    });

    // Send initial status
    broadcastStatus();
}

function handleAgentConnection(ws, connectionId, req) {
    // Agents use NDJSON protocol
    // Check Authorization header for security in production
    const authHeader = req.headers['authorization'];
    if (!authHeader) {
        console.warn(`Agent ${connectionId} connected without Authorization header`);
    }

    const agentState = {
        socket: ws,
        type: 'agent',
        id: connectionId,
        capabilities: {},
        status: 'connecting',
        buffer: ''
    };
    agents.set(connectionId, agentState);

    ws.on('message', (message) => {
        // Handle NDJSON stream - messages might come in chunks or multiple lines
        const text = message.toString();
        agentState.buffer += text;
        
        const lines = agentState.buffer.split('
');
        // Keep the last part if it doesn't end with newline
        agentState.buffer = lines.pop(); 

        for (const line of lines) {
            if (line.trim()) {
                try {
                    const data = JSON.parse(line);
                    handleAgentMessage(connectionId, data);
                } catch (e) {
                    console.error('Error parsing agent NDJSON:', e);
                }
            }
        }
    });

    ws.on('close', () => {
        console.log(`Agent disconnected: ${connectionId}`);
        agents.delete(connectionId);
        broadcastStatus();
    });

    // Send initial 'user' message to trigger 'system/init' from agent? 
    // According to protocol, Server sends 'user', CLI responds 'system/init'.
    // However, usually we wait for the Agent to identify itself.
    // Let's send a dummy user message to kickstart the handshake if required by protocol.
    // But typically, we wait for the TUI to assign work. 
    // For now, we just mark it as connected.
    broadcastStatus();
}

// --- Message Handlers ---

function handleClientMessage(clientId, data) {
    console.log(`Client ${clientId} sent:`, data.type);
    
    switch (data.type) {
        case 'list_agents':
            broadcastStatus();
            break;
        
        case 'submit_prompt':
            // { type: 'submit_prompt', prompt: '...', agentId: '...' }
            const targetAgent = agents.get(data.agentId);
            if (targetAgent) {
                sendToAgent(targetAgent, {
                    type: 'user',
                    text: data.prompt
                });
                targetAgent.status = 'busy';
                broadcastStatus();
            } else {
                sendToClient(clientId, { type: 'error', message: 'Agent not found' });
            }
            break;

        case 'start_debate':
            // { type: 'start_debate', topic: '...', agentA: '...', agentB: '...' }
            startDebate(data.topic, data.agentA, data.agentB);
            break;

        case 'forward_to_agent':
            // { type: 'forward_to_agent', agentId: '...', payload: { ... } }
            const agent = agents.get(data.agentId);
            if (agent) {
                console.log(`Forwarding control message to agent ${data.agentId}:`, data.payload);
                sendToAgent(agent, data.payload);
                sendToClient(clientId, { type: 'success', message: 'Message forwarded' });
            } else {
                sendToClient(clientId, { type: 'error', message: 'Agent not found' });
            }
            break;
            
        default:
            console.warn('Unknown client message type:', data.type);
    }
}

function handleAgentMessage(agentId, data) {
    const agent = agents.get(agentId);
    if (!agent) return;

    // Broadcast raw events to clients for debugging/streaming
    broadcastToClients({
        type: 'agent_event',
        agentId: agentId,
        payload: data
    });

    switch (data.type) {
        case 'system/init':
            console.log(`Agent ${agentId} initialized.`);
            agent.status = 'idle';
            broadcastStatus();
            break;

        case 'assistant':
            // Final response from agent
            console.log(`Agent ${agentId} finished response.`);
            agent.status = 'idle';
            checkDebateTurn(agentId, data.text); // Check if this was part of a debate
            broadcastStatus();
            break;
            
        case 'stream_event':
            // Streaming content (if verbose)
            break;
            
        case 'tool_use':
            console.log(`Agent ${agentId} using tool:`, data.tool);
            // In a real implementation, we might need to approve this
            // or the agent might be waiting for tool output.
            break;
    }
}

// --- Orchestration Logic ---

function startDebate(topic, agentAId, agentBId) {
    const agentA = agents.get(agentAId);
    const agentB = agents.get(agentBId);

    if (!agentA || !agentB) {
        console.error('Cannot start debate: Agents not connected');
        return;
    }

    const debateId = uuidv4();
    activeDebates.set(debateId, {
        id: debateId,
        topic: topic,
        agentA: agentAId,
        agentB: agentBId,
        turn: 'A', // A starts
        round: 1,
        maxRounds: 3
    });

    console.log(`Starting Debate ${debateId}: ${topic}`);
    
    // Initial Prompt to Agent A
    const prompt = `Topic: ${topic}

You are arguing FOR this topic. Please provide your opening statement.`;
    sendToAgent(agentA, { type: 'user', text: prompt });
    agentA.status = 'debating';
    agentB.status = 'debating'; // B waits
    broadcastStatus();
}

function checkDebateTurn(agentId, responseText) {
    // Find active debate involving this agent
    let activeDebate = null;
    for (const debate of activeDebates.values()) {
        if ((debate.agentA === agentId && debate.turn === 'A') || 
            (debate.agentB === agentId && debate.turn === 'B')) {
            activeDebate = debate;
            break;
        }
    }

    if (!activeDebate) return;

    const debate = activeDebate;
    
    // Switch turn
    if (debate.turn === 'A') {
        debate.turn = 'B';
        const agentB = agents.get(debate.agentB);
        if (agentB) {
            const prompt = `Your opponent argued:
"${responseText}"

You are arguing AGAINST the original topic: "${debate.topic}".
Please provide your rebuttal.`;
            sendToAgent(agentB, { type: 'user', text: prompt });
        }
    } else {
        // Turn was B, back to A or finish
        debate.turn = 'A';
        debate.round++;
        
        if (debate.round > debate.maxRounds) {
            console.log(`Debate ${debate.id} finished.`);
            activeDebates.delete(debate.id);
            const agentA = agents.get(debate.agentA);
            const agentB = agents.get(debate.agentB);
            if (agentA) agentA.status = 'idle';
            if (agentB) agentB.status = 'idle';
            broadcastToClients({ type: 'debate_finished', debateId: debate.id });
        } else {
            const agentA = agents.get(debate.agentA);
            if (agentA) {
                const prompt = `Your opponent argued:
"${responseText}"

Please defend your position and counter their points.`;
                sendToAgent(agentA, { type: 'user', text: prompt });
            }
        }
    }
}

// --- Helpers ---

function sendToAgent(agent, messageObj) {
    if (agent && agent.socket.readyState === WebSocket.OPEN) {
        agent.socket.send(JSON.stringify(messageObj) + '
'); // NDJSON
    }
}

function sendToClient(clientId, messageObj) {
    const client = clients.get(clientId);
    if (client && client.socket.readyState === WebSocket.OPEN) {
        client.socket.send(JSON.stringify(messageObj));
    }
}

function broadcastToClients(messageObj) {
    const msg = JSON.stringify(messageObj);
    for (const client of clients.values()) {
        if (client.socket.readyState === WebSocket.OPEN) {
            client.socket.send(msg);
        }
    }
}

function broadcastStatus() {
    const status = {
        type: 'status_update',
        agents: Array.from(agents.values()).map(a => ({
            id: a.id,
            status: a.status
        })),
        debates: Array.from(activeDebates.values())
    };
    broadcastToClients(status);
}
