<template>
  <div class="server-status">
    <div class="status-card">
      <h2>📊 Server Information</h2>
      
      <div class="info-grid">
        <div class="info-item">
          <span class="label">Container Name:</span>
          <span class="value">{{ status.container_name || 'N/A' }}</span>
        </div>
        
        <div class="info-item">
          <span class="label">Hostname:</span>
          <span class="value">{{ status.hostname || 'N/A' }}</span>
        </div>
        
        <div class="info-item">
          <span class="label">Image:</span>
          <span class="value">{{ status.image_name || 'docker-mailserver' }}</span>
        </div>
        
        <div class="info-item">
          <span class="label">Status:</span>
          <span :class="['value', 'status', status.is_running ? 'running' : 'stopped']">
            {{ status.is_running ? '🟢 Running' : '🔴 Stopped' }}
          </span>
        </div>
      </div>
    </div>

    <div class="status-card">
      <h2>🔌 Ports</h2>
      
      <div class="ports-grid">
        <div class="port-item">
          <span class="port-label">SMTP:</span>
          <span class="port-value">{{ status.smtp_port || 25 }}</span>
        </div>
        <div class="port-item">
          <span class="port-label">Submission:</span>
          <span class="port-value">{{ status.submission_port || 587 }}</span>
        </div>
        <div class="port-item">
          <span class="port-label">SMTPS:</span>
          <span class="port-value">{{ status.smtps_port || 465 }}</span>
        </div>
        <div class="port-item">
          <span class="port-label">IMAP:</span>
          <span class="port-value">{{ status.imap_port || 143 }}</span>
        </div>
        <div class="port-item">
          <span class="port-label">IMAPS:</span>
          <span class="port-value">{{ status.imaps_port || 993 }}</span>
        </div>
        <div class="port-item">
          <span class="port-label">POP3:</span>
          <span class="port-value">{{ status.pop3_port || 110 }}</span>
        </div>
      </div>
    </div>

    <div class="status-card">
      <h2>⚙️ Features</h2>
      
      <div class="features-grid">
        <div class="feature-item">
          <span class="feature-icon">{{ status.spam_enabled ? '✅' : '❌' }}</span>
          <span>Spam Protection</span>
        </div>
        <div class="feature-item">
          <span class="feature-icon">{{ status.virus_enabled ? '✅' : '❌' }}</span>
          <span>Virus Scanning</span>
        </div>
        <div class="feature-item">
          <span class="feature-icon">{{ status.dkim_enabled ? '✅' : '❌' }}</span>
          <span>DKIM Signing</span>
        </div>
        <div class="feature-item">
          <span class="feature-icon">{{ status.ssl_enabled ? '✅' : '❌' }}</span>
          <span>SSL/TLS</span>
        </div>
      </div>
    </div>

    <button @click="$emit('refresh')" class="btn-refresh">
      🔄 Refresh Status
    </button>
  </div>
</template>

<script>
export default {
  name: 'ServerStatus',
  props: {
    status: {
      type: Object,
      default: () => ({})
    }
  }
};
</script>

<style scoped>
.server-status {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.status-card {
  background: white;
  border-radius: 12px;
  padding: 24px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.1);
}

.status-card h2 {
  margin: 0 0 20px 0;
  font-size: 1.3rem;
  color: #333;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 16px;
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.label {
  font-size: 13px;
  color: #666;
  font-weight: 500;
}

.value {
  font-size: 16px;
  color: #333;
  font-weight: 600;
}

.value.status.running {
  color: #10b981;
}

.value.status.stopped {
  color: #ef4444;
}

.ports-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 16px;
}

.port-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px;
  background: #f9fafb;
  border-radius: 8px;
}

.port-label {
  font-size: 14px;
  color: #666;
  font-weight: 500;
}

.port-value {
  font-size: 16px;
  color: #3b82f6;
  font-weight: 700;
  font-family: 'Courier New', monospace;
}

.features-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
}

.feature-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px;
  background: #f9fafb;
  border-radius: 8px;
  font-size: 15px;
  color: #333;
}

.feature-icon {
  font-size: 20px;
}

.btn-refresh {
  padding: 12px 24px;
  background: #3b82f6;
  color: white;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-size: 15px;
  font-weight: 600;
  transition: background 0.2s;
  align-self: flex-start;
}

.btn-refresh:hover {
  background: #2563eb;
}
</style>
