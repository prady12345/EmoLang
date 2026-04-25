// ── State ──────────────────────────────────────────────────────────
let socket = null;
let myUsername = '';
let myRoomCode = '';

// ── DOM references ──────────────────────────────────────────────────
const homeScreen    = document.getElementById('home-screen');
const chatScreen    = document.getElementById('chat-screen');
const usernameInput = document.getElementById('username-input');
const joinCodeInput = document.getElementById('join-code-input');
const createBtn     = document.getElementById('create-btn');
const joinBtn       = document.getElementById('join-btn');
const messagesDiv   = document.getElementById('messages');
const msgInput      = document.getElementById('msg-input');
const sendBtn       = document.getElementById('send-btn');
const copyLinkBtn   = document.getElementById('copy-link-btn');
const copyLabel     = document.getElementById('copy-label');
const roomCodeDisplay = document.getElementById('room-code-display');

// ── Mobile keyboard scroll fix ──────────────────────────────────────
// When the virtual keyboard opens/closes on mobile, dvh changes.
// Re-scroll messages to bottom so the latest message stays visible.
if ('visualViewport' in window) {
  window.visualViewport.addEventListener('resize', () => {
    scrollToBottom();
  });
}

function scrollToBottom() {
  messagesDiv.scrollTop = messagesDiv.scrollHeight;
}

// ── Room creation ───────────────────────────────────────────────────
createBtn.addEventListener('click', async () => {
  const username = usernameInput.value.trim() || 'Anonymous';

  createBtn.textContent = 'Creating...';
  createBtn.disabled = true;

  try {
    const res = await fetch('/api/create-room', { method: 'POST' });
    const data = await res.json();
    enterRoom(data.code, username);
  } catch (e) {
    alert('Could not reach server. Is it running?');
  } finally {
    createBtn.textContent = 'Create a room';
    createBtn.disabled = false;
  }
});

// ── Room joining ────────────────────────────────────────────────────
joinBtn.addEventListener('click', async () => {
  const code = joinCodeInput.value.trim().toUpperCase();
  const username = usernameInput.value.trim() || 'Anonymous';

  if (!code || code.length < 4) {
    joinCodeInput.focus();
    return;
  }

  joinBtn.textContent = 'Joining...';
  joinBtn.disabled = true;

  try {
    const res = await fetch(`/api/room/${code}`);
    if (!res.ok) { alert('Room not found. Check the code.'); return; }
    enterRoom(code, username);
  } catch (e) {
    alert('Could not reach server.');
  } finally {
    joinBtn.textContent = 'Join';
    joinBtn.disabled = false;
  }
});

// ── Auto-join from shareable link ───────────────────────────────────
window.addEventListener('DOMContentLoaded', () => {
  const match = window.location.pathname.match(/^\/room\/([A-Z0-9]{4,8})$/i);
  if (match) {
    const code = match[1].toUpperCase();
    const name = prompt('What\'s your name?') || 'Anonymous';
    usernameInput.value = name;
    joinCodeInput.value = code;
    joinBtn.click();
  }
});

// ── Enter a room ────────────────────────────────────────────────────
function enterRoom(code, username) {
  myUsername = username;
  myRoomCode = code;

  history.pushState({}, '', `/room/${code}`);

  roomCodeDisplay.textContent = code;

  homeScreen.classList.add('hidden');
  chatScreen.classList.remove('hidden');

  // Focus message input — on desktop this is instant;
  // on mobile we delay to avoid fighting with the layout shift
  setTimeout(() => msgInput.focus(), 300);

  connectWebSocket(code, username);
}

// ── WebSocket ───────────────────────────────────────────────────────
function connectWebSocket(code, username) {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const url = `${proto}://${location.host}/ws/${code}?username=${encodeURIComponent(username)}`;

  socket = new WebSocket(url);

  socket.onopen = () => {
    appendMessage({ type: 'system', text: 'Connected.' });
  };

  socket.onmessage = (event) => {
    const data = JSON.parse(event.data);
    appendMessage(data);
  };

  socket.onclose = () => {
    appendMessage({ type: 'system', text: 'Disconnected.' });
  };

  socket.onerror = () => {
    appendMessage({ type: 'system', text: 'Connection error.' });
  };
}

// ── Send a message ──────────────────────────────────────────────────
function sendMessage() {
  const text = msgInput.value.trim();
  if (!text || !socket || socket.readyState !== WebSocket.OPEN) return;

  socket.send(JSON.stringify(text));
  msgInput.value = '';

  // On mobile, keep keyboard open after sending
  msgInput.focus();
}

sendBtn.addEventListener('click', sendMessage);

msgInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendMessage();
  }
});

// ── Render a message bubble ─────────────────────────────────────────
function appendMessage(data) {
  const div = document.createElement('div');

  if (data.type === 'system') {
    div.className = 'msg system';
    div.textContent = data.text;
  } else {
    const isMine = data.username === myUsername;
    div.className = `msg ${isMine ? 'mine' : 'other'}`;

    if (!isMine) {
      const sender = document.createElement('div');
      sender.className = 'sender';
      sender.textContent = data.username;
      div.appendChild(sender);
    }

    const textNode = document.createElement('div');
    textNode.textContent = data.text;
    div.appendChild(textNode);
  }

  messagesDiv.appendChild(div);
  scrollToBottom();
}

// ── Copy shareable link ─────────────────────────────────────────────
copyLinkBtn.addEventListener('click', () => {
  const link = `${location.origin}/room/${myRoomCode}`;

  // navigator.share is the native mobile share sheet (iOS/Android)
  if (navigator.share) {
    navigator.share({
      title: 'Join my ChatRoom',
      text: `Join my chat room with code ${myRoomCode}`,
      url: link,
    }).catch(() => {});
    return;
  }

  // Desktop fallback: copy to clipboard
  navigator.clipboard.writeText(link).then(() => {
    copyLabel.textContent = 'Copied!';
    setTimeout(() => copyLabel.textContent = 'Share', 2000);
  });
});