/* opsmate Chat UI — WebSocket Client */

(function () {
  'use strict';

  // ─── Config ─────────────────────────────────────────────────────
  const WS_URL = `ws://${window.location.host}/ws`;
  const RECONNECT_BASE_MS = 1000;
  const RECONNECT_MAX_MS  = 30000;
  const MAX_INPUT_ROWS    = 5;

  // ─── State ───────────────────────────────────────────────────────
  let ws               = null;
  let sessionId        = null;
  let isStreaming      = false;
  let reconnectDelay   = RECONNECT_BASE_MS;
  let reconnectTimer   = null;
  let currentAssistantEl = null;  // live streaming bubble
  let userScrolledUp   = false;
  let lastScrollTop    = 0;

  // ─── DOM refs ────────────────────────────────────────────────────
  const messagesContainer = document.getElementById('messagesContainer');
  const messageInput      = document.getElementById('messageInput');
  const sendBtn           = document.getElementById('sendBtn');
  const typingIndicator   = document.getElementById('typingIndicator');
  const statusDot         = document.getElementById('statusDot');
  const welcomeMessage    = document.getElementById('welcomeMessage');

  // ─── marked.js setup ─────────────────────────────────────────────
  marked.setOptions({
    gfm: true,
    breaks: true,
    highlight: function (code, lang) {
      if (lang && hljs.getLanguage(lang)) {
        try {
          return hljs.highlight(code, { language: lang }).value;
        } catch (e) {}
      }
      return hljs.highlightAuto(code).value;
    },
  });

  // Custom renderer: open links in new tab
  const renderer = new marked.Renderer();
  renderer.link = function (href, title, text) {
    return `<a href="${href}" target="_blank" rel="noopener noreferrer"${title ? ` title="${title}"` : ''}>${text}</a>`;
  };
  marked.setOptions({ renderer });

  // ─── WebSocket lifecycle ──────────────────────────────────────────

  function connect() {
    setStatus('connecting');
    ws = new WebSocket(WS_URL);

    ws.addEventListener('open', onOpen);
    ws.addEventListener('message', onMessage);
    ws.addEventListener('close', onClose);
    ws.addEventListener('error', onError);
  }

  function onOpen() {
    reconnectDelay = RECONNECT_BASE_MS;
    setStatus('connected');

    // Resume session if we have one
    if (sessionId) {
      send({ type: 'resume', session_id: sessionId });
    }
  }

  function onClose(evt) {
    setStatus('disconnected');
    ws = null;
    finalizeCurrentAssistant();
    scheduleReconnect();
  }

  function onError(evt) {
    setStatus('error');
  }

  function scheduleReconnect() {
    if (reconnectTimer) return;
    reconnectTimer = setTimeout(function () {
      reconnectTimer = null;
      connect();
    }, reconnectDelay);
    reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX_MS);
  }

  // ─── Message handling ─────────────────────────────────────────────

  function onMessage(evt) {
    let data;
    try {
      data = JSON.parse(evt.data);
    } catch (e) {
      console.error('Invalid JSON from server:', evt.data);
      return;
    }

    switch (data.type) {
      case 'assistant_chunk':
        handleAssistantChunk(data.content || '');
        break;

      case 'tool_use':
        handleToolUse(data.tool || data.name || 'tool', data.input);
        break;

      case 'message_end':
        if (data.session_id) {
          sessionId = data.session_id;
        }
        finalizeCurrentAssistant();
        break;

      case 'error':
        finalizeCurrentAssistant();
        appendError(data.message || 'An error occurred.');
        break;

      default:
        console.warn('Unknown message type:', data.type);
    }
  }

  // Streaming: accumulate raw markdown in dataset, re-render each chunk
  let _streamBuffer = '';

  function handleAssistantChunk(chunk) {
    hideWelcome();
    showTyping();

    if (!currentAssistantEl) {
      _streamBuffer = '';
      currentAssistantEl = createAssistantBubble();
    }

    _streamBuffer += chunk;
    currentAssistantEl.innerHTML = marked.parse(_streamBuffer);

    // Re-highlight new code blocks
    currentAssistantEl.querySelectorAll('pre code:not(.hljs)').forEach(function (el) {
      hljs.highlightElement(el);
    });

    maybeScrollToBottom();
  }

  function handleToolUse(toolName, inputData) {
    hideWelcome();

    const row = document.createElement('div');
    row.className = 'tool-row';

    const badge = document.createElement('span');
    badge.className = 'tool-badge';

    let inputStr = '';
    if (inputData) {
      try {
        const parsed = typeof inputData === 'string' ? JSON.parse(inputData) : inputData;
        const keys = Object.keys(parsed).slice(0, 2);
        inputStr = keys.map(function (k) { return `${k}=${JSON.stringify(parsed[k])}`; }).join(', ');
      } catch (e) {
        inputStr = String(inputData).slice(0, 60);
      }
    }

    badge.innerHTML = `<span class="tool-icon">⚙</span>${toolName}${inputStr ? ` <span style="color:var(--text-muted)">${inputStr}</span>` : ''}`;
    row.appendChild(badge);
    messagesContainer.appendChild(row);
    maybeScrollToBottom();
  }

  function finalizeCurrentAssistant() {
    hideTyping();
    currentAssistantEl = null;
    _streamBuffer = '';
    setStreaming(false);
  }

  function appendError(msg) {
    const el = document.createElement('div');
    el.className = 'error-message';
    el.textContent = '⚠ ' + msg;
    messagesContainer.appendChild(el);
    maybeScrollToBottom();
  }

  // ─── DOM helpers ─────────────────────────────────────────────────

  function createUserBubble(text) {
    hideWelcome();
    const row = document.createElement('div');
    row.className = 'message user';

    const label = document.createElement('div');
    label.className = 'message-label';
    label.textContent = 'you';

    const bubble = document.createElement('div');
    bubble.className = 'message-bubble';
    bubble.textContent = text;

    row.appendChild(label);
    row.appendChild(bubble);
    messagesContainer.appendChild(row);
    scrollToBottom();
    return bubble;
  }

  function createAssistantBubble() {
    const row = document.createElement('div');
    row.className = 'message assistant';

    const label = document.createElement('div');
    label.className = 'message-label';
    label.textContent = 'opsmate';

    const bubble = document.createElement('div');
    bubble.className = 'message-bubble';

    row.appendChild(label);
    row.appendChild(bubble);
    messagesContainer.appendChild(row);
    return bubble;
  }

  function hideWelcome() {
    if (welcomeMessage) {
      welcomeMessage.style.display = 'none';
    }
  }

  function showTyping() {
    isStreaming = true;
    typingIndicator.classList.add('visible');
  }

  function hideTyping() {
    typingIndicator.classList.remove('visible');
  }

  function setStatus(state) {
    statusDot.className = 'status-dot';
    if (state === 'connected') {
      statusDot.classList.add('connected');
      statusDot.title = 'Connected';
    } else if (state === 'connecting') {
      statusDot.classList.add('connecting');
      statusDot.title = 'Connecting…';
    } else if (state === 'error') {
      statusDot.classList.add('error');
      statusDot.title = 'Error';
    } else {
      statusDot.title = 'Disconnected';
    }
  }

  function setStreaming(active) {
    isStreaming = active;
    messageInput.disabled = active;
    if (active) {
      sendBtn.title = 'Stop';
      sendBtn.classList.add('stop');
      sendBtn.innerHTML = `<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><rect x="4" y="4" width="16" height="16" rx="2"/></svg>`;
    } else {
      sendBtn.title = 'Send (Enter)';
      sendBtn.classList.remove('stop');
      sendBtn.innerHTML = `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="22" y1="2" x2="11" y2="13"></line><polygon points="22 2 15 22 11 13 2 9 22 2"></polygon></svg>`;
    }
  }

  // ─── Scroll management ────────────────────────────────────────────

  messagesContainer.addEventListener('scroll', function () {
    const scrollBottom = messagesContainer.scrollHeight - messagesContainer.scrollTop - messagesContainer.clientHeight;
    userScrolledUp = scrollBottom > 80;
  });

  function scrollToBottom() {
    messagesContainer.scrollTop = messagesContainer.scrollHeight;
  }

  function maybeScrollToBottom() {
    if (!userScrolledUp) {
      scrollToBottom();
    }
  }

  // ─── Send message ─────────────────────────────────────────────────

  function sendMessage() {
    const text = messageInput.value.trim();
    if (!text) return;

    if (isStreaming) {
      // Act as stop button
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.close();
      }
      finalizeCurrentAssistant();
      return;
    }

    if (!ws || ws.readyState !== WebSocket.OPEN) {
      appendError('Not connected. Reconnecting…');
      connect();
      return;
    }

    createUserBubble(text);
    messageInput.value = '';
    resizeInput();
    setStreaming(true);

    send({ type: 'user_message', content: text });
  }

  function send(obj) {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(obj));
    }
  }

  // ─── Input auto-resize ─────────────────────────────────────────────

  function resizeInput() {
    messageInput.style.height = 'auto';
    const lineHeight = parseFloat(getComputedStyle(messageInput).lineHeight);
    const maxHeight  = lineHeight * MAX_INPUT_ROWS;
    const newHeight  = Math.min(messageInput.scrollHeight, maxHeight);
    messageInput.style.height = newHeight + 'px';
    messageInput.style.overflowY = messageInput.scrollHeight > maxHeight ? 'auto' : 'hidden';
  }

  messageInput.addEventListener('input', resizeInput);

  messageInput.addEventListener('keydown', function (evt) {
    if (evt.key === 'Enter' && !evt.shiftKey) {
      evt.preventDefault();
      sendMessage();
    }
  });

  sendBtn.addEventListener('click', sendMessage);

  // ─── Bootstrap ───────────────────────────────────────────────────
  connect();
})();
