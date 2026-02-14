const WebSocket = require('ws');

const ws = new WebSocket('ws://localhost:3000?role=client');

ws.on('open', () => {
    console.log('Connected to SDK Server');
    
    // List agents
    ws.send(JSON.stringify({ type: 'list_agents' }));
    
    // Simulate a debate start (would fail if no agents connected, but tests the path)
    // ws.send(JSON.stringify({ 
    //     type: 'start_debate', 
    //     topic: 'AI Safety', 
    //     agentA: 'dummy-uuid-1', 
    //     agentB: 'dummy-uuid-2' 
    // }));
});

ws.on('message', (data) => {
    console.log('Received:', data.toString());
});

ws.on('close', () => {
    console.log('Disconnected');
});
