<template>
  <div class="webmail">
    <!-- Mailbox Selector -->
    <div class="mailbox-selector">
      <select v-model="selectedMailboxId" @change="loadEmails" class="mailbox-select">
        <option value="">Select Mailbox</option>
        <option v-for="mailbox in mailboxes" :key="mailbox.ID" :value="mailbox.ID">
          {{ mailbox.email }}
        </option>
      </select>
      <button @click="showComposer = true" class="btn-compose" :disabled="!selectedMailboxId">
        ✏️ Compose
      </button>
    </div>

    <div v-if="!selectedMailboxId" class="empty-state">
      <p>📬 Select a mailbox to view emails</p>
    </div>

    <div v-else class="webmail-layout">
      <!-- Sidebar -->
      <div class="sidebar">
        <div 
          v-for="folder in folders" 
          :key="folder.id"
          :class="['folder-item', { active: selectedFolder === folder.id }]"
          @click="selectFolder(folder.id)"
        >
          <span class="folder-icon">{{ folder.icon }}</span>
          <span class="folder-name">{{ folder.name }}</span>
          <span v-if="folder.count" class="folder-count">{{ folder.count }}</span>
        </div>
      </div>

      <!-- Email List -->
      <div class="email-list">
        <div class="list-header">
          <h3>{{ getCurrentFolderName() }}</h3>
          <button @click="loadEmails" class="btn-refresh">🔄</button>
        </div>

        <div v-if="loading" class="loading-state">
          <p>⏳ Loading emails...</p>
        </div>

        <div v-else-if="emails.length === 0" class="empty-emails">
          <p>📭 No emails in this folder</p>
        </div>

        <div v-else class="emails-container">
          <div 
            v-for="email in emails" 
            :key="email.ID"
            :class="['email-item', { unread: !email.seen, selected: selectedEmail?.ID === email.ID }]"
            @click="selectEmail(email)"
          >
            <div class="email-from">{{ email.from }}</div>
            <div class="email-subject">{{ email.subject || '(No Subject)' }}</div>
            <div class="email-preview">{{ email.body_plain?.substring(0, 100) }}</div>
            <div class="email-date">{{ formatDate(email.date) }}</div>
          </div>
        </div>
      </div>

      <!-- Email Viewer -->
      <div class="email-viewer" v-if="selectedEmail">
        <div class="viewer-header">
          <h2>{{ selectedEmail.subject || '(No Subject)' }}</h2>
          <div class="viewer-actions">
            <button @click="replyEmail" class="btn-action">↩️ Reply</button>
            <button @click="deleteEmail" class="btn-action btn-delete">🗑️ Delete</button>
          </div>
        </div>

        <div class="viewer-meta">
          <div class="meta-row">
            <span class="meta-label">From:</span>
            <span class="meta-value">{{ selectedEmail.from }}</span>
          </div>
          <div class="meta-row">
            <span class="meta-label">To:</span>
            <span class="meta-value">{{ selectedEmail.to }}</span>
          </div>
          <div v-if="selectedEmail.cc" class="meta-row">
            <span class="meta-label">CC:</span>
            <span class="meta-value">{{ selectedEmail.cc }}</span>
          </div>
          <div class="meta-row">
            <span class="meta-label">Date:</span>
            <span class="meta-value">{{ formatDateTime(selectedEmail.date) }}</span>
          </div>
        </div>

        <div class="viewer-body">
          <div v-if="selectedEmail.body_html" v-html="selectedEmail.body_html" class="email-html"></div>
          <pre v-else class="email-plain">{{ selectedEmail.body_plain }}</pre>
        </div>

        <div v-if="selectedEmail.has_attachments" class="attachments">
          <h4>📎 Attachments ({{ selectedEmail.attachment_count }})</h4>
          <p class="text-muted">Attachments feature coming soon...</p>
        </div>
      </div>

      <div v-else class="email-viewer empty">
        <p>📧 Select an email to read</p>
      </div>
    </div>

    <!-- Composer Modal -->
    <div v-if="showComposer" class="composer-modal" @click.self="showComposer = false">
      <div class="composer">
        <div class="composer-header">
          <h3>✏️ New Message</h3>
          <button @click="showComposer = false" class="btn-close">✕</button>
        </div>

        <form @submit.prevent="handleSend" class="composer-form">
          <input
            v-model="newEmail.to"
            type="email"
            placeholder="To:"
            required
            class="composer-input"
            multiple
          />
          <input
            v-model="newEmail.subject"
            type="text"
            placeholder="Subject:"
            required
            class="composer-input"
          />
          <textarea
            v-model="newEmail.body"
            placeholder="Write your message..."
            class="composer-textarea"
            rows="15"
          ></textarea>

          <div class="composer-actions">
            <button type="submit" class="btn-send" :disabled="sending">
              {{ sending ? '📤 Sending...' : '📤 Send' }}
            </button>
            <button type="button" @click="showComposer = false" class="btn-cancel">
              Cancel
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script>
import axios from 'axios';

export default {
  name: 'Webmail',
  props: {
    mailboxes: {
      type: Array,
      default: () => []
    }
  },
  data() {
    return {
      selectedMailboxId: '',
      selectedFolder: 'INBOX',
      selectedEmail: null,
      emails: [],
      loading: false,
      showComposer: false,
      sending: false,
      newEmail: {
        to: '',
        subject: '',
        body: ''
      },
      folders: [
        { id: 'INBOX', name: 'Inbox', icon: '📥', count: 0 },
        { id: 'Sent', name: 'Sent', icon: '📤', count: 0 },
        { id: 'Drafts', name: 'Drafts', icon: '📝', count: 0 },
        { id: 'Spam', name: 'Spam', icon: '🚫', count: 0 },
        { id: 'Trash', name: 'Trash', icon: '🗑️', count: 0 },
      ]
    };
  },
  methods: {
    async loadEmails() {
      if (!this.selectedMailboxId) return;
      
      this.loading = true;
      try {
        const response = await axios.get(
          `/api/email/mailboxes/${this.selectedMailboxId}/emails`,
          { params: { folder: this.selectedFolder, limit: 50 } }
        );
        if (!response.data.error) {
          this.emails = response.data.data || [];
        }
      } catch (error) {
        console.error('Failed to load emails:', error);
      } finally {
        this.loading = false;
      }
    },
    
    selectFolder(folderId) {
      this.selectedFolder = folderId;
      this.selectedEmail = null;
      this.loadEmails();
    },
    
    selectEmail(email) {
      this.selectedEmail = email;
    },
    
    getCurrentFolderName() {
      const folder = this.folders.find(f => f.id === this.selectedFolder);
      return folder ? folder.name : 'Folder';
    },
    
    async handleSend() {
      this.sending = true;
      try {
        const emailData = {
          mailbox_id: parseInt(this.selectedMailboxId),
          to: this.newEmail.to.split(',').map(e => e.trim()),
          subject: this.newEmail.subject,
          body: this.newEmail.body
        };
        
        await this.$emit('send', emailData);
        
        this.showComposer = false;
        this.newEmail = { to: '', subject: '', body: '' };
      } catch (error) {
        console.error('Failed to send email:', error);
      } finally {
        this.sending = false;
      }
    },
    
    replyEmail() {
      this.newEmail.to = this.selectedEmail.from;
      this.newEmail.subject = 'Re: ' + this.selectedEmail.subject;
      this.showComposer = true;
    },
    
    async deleteEmail() {
      if (confirm('Delete this email?')) {
        // TODO: Implement delete
        this.$notify({ type: 'info', text: 'Delete feature coming soon...' });
      }
    },
    
    formatDate(date) {
      if (!date) return '';
      const d = new Date(date);
      const now = new Date();
      const diffDays = Math.floor((now - d) / (1000 * 60 * 60 * 24));
      
      if (diffDays === 0) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
      if (diffDays === 1) return 'Yesterday';
      if (diffDays < 7) return d.toLocaleDateString([], { weekday: 'short' });
      return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
    },
    
    formatDateTime(date) {
      if (!date) return '';
      return new Date(date).toLocaleString();
    }
  }
};
</script>

<style scoped>
.webmail {
  background: white;
  border-radius: 12px;
  padding: 24px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.1);
  min-height: 600px;
}

.mailbox-selector {
  display: flex;
  gap: 12px;
  margin-bottom: 20px;
}

.mailbox-select {
  flex: 1;
  padding: 12px;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  font-size: 15px;
}

.btn-compose {
  padding: 12px 24px;
  background: #3b82f6;
  color: white;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-weight: 600;
  transition: background 0.2s;
}

.btn-compose:hover:not(:disabled) {
  background: #2563eb;
}

.btn-compose:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.empty-state {
  text-align: center;
  padding: 60px 20px;
  color: #666;
  font-size: 18px;
}

.webmail-layout {
  display: grid;
  grid-template-columns: 200px 400px 1fr;
  gap: 20px;
  height: 700px;
}

/* Sidebar */
.sidebar {
  border-right: 2px solid #e0e0e0;
  padding-right: 16px;
}

.folder-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px;
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.2s;
  margin-bottom: 4px;
}

.folder-item:hover {
  background: #f3f4f6;
}

.folder-item.active {
  background: #dbeafe;
  color: #1e40af;
  font-weight: 600;
}

.folder-icon {
  font-size: 18px;
}

.folder-name {
  flex: 1;
}

.folder-count {
  font-size: 13px;
  font-weight: 700;
  color: #3b82f6;
}

/* Email List */
.email-list {
  border-right: 2px solid #e0e0e0;
  display: flex;
  flex-direction: column;
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.list-header h3 {
  margin: 0;
  font-size: 1.2rem;
}

.btn-refresh {
  padding: 6px 10px;
  background: #f3f4f6;
  border: none;
  border-radius: 6px;
  cursor: pointer;
}

.loading-state, .empty-emails {
  text-align: center;
  padding: 40px 20px;
  color: #666;
}

.emails-container {
  flex: 1;
  overflow-y: auto;
}

.email-item {
  padding: 12px;
  border-bottom: 1px solid #e0e0e0;
  cursor: pointer;
  transition: background 0.2s;
}

.email-item:hover {
  background: #f9fafb;
}

.email-item.unread {
  background: #eff6ff;
  font-weight: 600;
}

.email-item.selected {
  background: #dbeafe;
  border-left: 4px solid #3b82f6;
}

.email-from {
  font-size: 14px;
  font-weight: 600;
  color: #333;
  margin-bottom: 4px;
}

.email-subject {
  font-size: 14px;
  color: #333;
  margin-bottom: 4px;
}

.email-preview {
  font-size: 13px;
  color: #666;
  margin-bottom: 6px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.email-date {
  font-size: 12px;
  color: #999;
}

/* Email Viewer */
.email-viewer {
  overflow-y: auto;
  padding-left: 20px;
}

.email-viewer.empty {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #666;
  font-size: 18px;
}

.viewer-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 20px;
  padding-bottom: 16px;
  border-bottom: 2px solid #e0e0e0;
}

.viewer-header h2 {
  margin: 0;
  font-size: 1.5rem;
  color: #333;
}

.viewer-actions {
  display: flex;
  gap: 8px;
}

.btn-action {
  padding: 8px 16px;
  background: #f3f4f6;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  transition: background 0.2s;
}

.btn-action:hover {
  background: #e5e7eb;
}

.btn-delete:hover {
  background: #fee2e2;
  color: #dc2626;
}

.viewer-meta {
  margin-bottom: 24px;
  padding: 16px;
  background: #f9fafb;
  border-radius: 8px;
}

.meta-row {
  display: flex;
  margin-bottom: 8px;
  font-size: 14px;
}

.meta-label {
  min-width: 60px;
  color: #666;
  font-weight: 600;
}

.meta-value {
  color: #333;
}

.viewer-body {
  margin-bottom: 24px;
}

.email-html {
  padding: 20px;
  background: white;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
}

.email-plain {
  padding: 20px;
  background: #f9fafb;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  white-space: pre-wrap;
  font-family: 'Courier New', monospace;
  font-size: 14px;
}

.attachments {
  padding: 16px;
  background: #f9fafb;
  border-radius: 8px;
}

.attachments h4 {
  margin: 0 0 12px 0;
}

.text-muted {
  color: #999;
  font-size: 14px;
}

/* Composer Modal */
.composer-modal {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0,0,0,0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.composer {
  background: white;
  border-radius: 12px;
  width: 700px;
  max-width: 90%;
  max-height: 90vh;
  overflow: auto;
  box-shadow: 0 20px 60px rgba(0,0,0,0.3);
}

.composer-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px;
  border-bottom: 2px solid #e0e0e0;
}

.composer-header h3 {
  margin: 0;
  font-size: 1.3rem;
}

.btn-close {
  width: 32px;
  height: 32px;
  border: none;
  background: #f3f4f6;
  border-radius: 50%;
  cursor: pointer;
  font-size: 18px;
}

.btn-close:hover {
  background: #e5e7eb;
}

.composer-form {
  padding: 20px;
}

.composer-input {
  width: 100%;
  padding: 12px;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  font-size: 15px;
  margin-bottom: 12px;
}

.composer-input:focus {
  outline: none;
  border-color: #3b82f6;
}

.composer-textarea {
  width: 100%;
  padding: 12px;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  font-size: 15px;
  font-family: inherit;
  resize: vertical;
  margin-bottom: 16px;
}

.composer-textarea:focus {
  outline: none;
  border-color: #3b82f6;
}

.composer-actions {
  display: flex;
  gap: 12px;
}

.btn-send {
  padding: 12px 24px;
  background: #3b82f6;
  color: white;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-weight: 600;
  transition: background 0.2s;
}

.btn-send:hover:not(:disabled) {
  background: #2563eb;
}

.btn-send:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-cancel {
  padding: 12px 24px;
  background: #f3f4f6;
  color: #333;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-weight: 600;
  transition: background 0.2s;
}

.btn-cancel:hover {
  background: #e5e7eb;
}
</style>
