<template>
  <div class="mailbox-management">
    <!-- Add Mailbox Form -->
    <div class="add-form-card">
      <h2>➕ Create New Mailbox</h2>
      <form @submit.prevent="handleAdd" class="add-form">
        <div class="form-grid">
          <select v-model="newMailbox.domain_id" required class="form-select">
            <option value="">Select Domain</option>
            <option v-for="domain in domains" :key="domain.ID" :value="domain.ID">
              {{ domain.domain }}
            </option>
          </select>
          
          <input
            v-model="newMailbox.username"
            type="text"
            placeholder="Username (without @domain)"
            required
            class="form-input"
          />
          
          <input
            v-model="newMailbox.password"
            type="password"
            placeholder="Password"
            required
            class="form-input"
          />
          
          <input
            v-model="newMailbox.name"
            type="text"
            placeholder="Display Name"
            class="form-input"
          />
        </div>
        <button type="submit" class="btn-add">Create Mailbox</button>
      </form>
    </div>

    <!-- Mailboxes List -->
    <div class="mailboxes-card">
      <div class="card-header">
        <h2>📬 Mailboxes ({{ mailboxes.length }})</h2>
        <button @click="$emit('refresh')" class="btn-icon">🔄</button>
      </div>

      <div v-if="mailboxes.length === 0" class="empty-state">
        <p>📭 No mailboxes yet. Create your first mailbox above!</p>
      </div>

      <div v-else class="mailboxes-grid">
        <div v-for="mailbox in mailboxes" :key="mailbox.ID" class="mailbox-card">
          <div class="mailbox-header">
            <div class="mailbox-avatar">
              {{ getInitials(mailbox.name || mailbox.username) }}
            </div>
            <div class="mailbox-info">
              <h3>{{ mailbox.name || mailbox.username }}</h3>
              <p class="mailbox-email">{{ mailbox.email }}</p>
            </div>
            <span :class="['status-dot', mailbox.enabled ? 'enabled' : 'disabled']" />
          </div>

          <div class="mailbox-stats">
            <div class="stat-item">
              <span class="stat-label">Messages</span>
              <span class="stat-value">{{ mailbox.message_count || 0 }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-label">Quota</span>
              <span class="stat-value">{{ formatQuota(mailbox.used_quota, mailbox.quota) }}</span>
            </div>
            <div class="stat-item">
              <span class="stat-label">Last Login</span>
              <span class="stat-value">{{ formatDate(mailbox.last_login) }}</span>
            </div>
          </div>

          <div class="mailbox-actions">
            <button class="btn-action btn-webmail" @click="openWebmail(mailbox)">
              ✉️ Open Webmail
            </button>
            <button class="btn-action btn-settings" @click="openEditModal(mailbox)">
              ⚙️ Settings
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Edit Mailbox Modal -->
    <div v-if="showEditModal" class="modal-overlay" @click="closeEditModal">
      <div class="modal-content" @click.stop>
        <div class="modal-header">
          <h3>⚙️ Mailbox Settings: {{ editingMailbox.email }}</h3>
          <button @click="closeEditModal" class="btn-close">✕</button>
        </div>
        
        <form @submit.prevent="handleEdit" class="edit-form">
          <div class="form-group">
            <label>Display Name</label>
            <input
              v-model="editForm.name"
              type="text"
              placeholder="John Doe"
              class="form-input"
            />
          </div>
          
          <div class="form-group">
            <label>Quota (bytes)</label>
            <input
              v-model.number="editForm.quota"
              type="number"
              placeholder="10737418240"
              class="form-input"
            />
            <small class="form-hint">Default: 10GB = 10737418240 bytes</small>
          </div>
          
          <div class="form-group">
            <label>Forward To (optional)</label>
            <input
              v-model="editForm.forward_to"
              type="email"
              placeholder="forward@example.com"
              class="form-input"
            />
          </div>
          
          <div class="form-group">
            <label>Auto Reply Message (optional)</label>
            <textarea
              v-model="editForm.auto_reply_msg"
              placeholder="I'm out of office..."
              class="form-textarea"
              rows="3"
            />
          </div>
          
          <div class="form-group">
            <label>New Password (optional)</label>
            <input
              v-model="editForm.password"
              type="password"
              placeholder="Leave empty to keep current"
              class="form-input"
            />
          </div>
          
          <div class="form-group">
            <label class="checkbox-label">
              <input
                v-model="editForm.enabled"
                type="checkbox"
                class="form-checkbox"
              />
              <span>Enable Mailbox</span>
            </label>
          </div>
          
          <div class="form-group">
            <label class="checkbox-label">
              <input
                v-model="editForm.keep_copy"
                type="checkbox"
                class="form-checkbox"
              />
              <span>Keep Copy When Forwarding</span>
            </label>
          </div>
          
          <div class="form-group">
            <label class="checkbox-label">
              <input
                v-model="editForm.auto_reply"
                type="checkbox"
                class="form-checkbox"
              />
              <span>Enable Auto Reply</span>
            </label>
          </div>
          
          <div class="modal-footer">
            <button type="button" @click="closeEditModal" class="btn-cancel">
              Cancel
            </button>
            <button type="submit" class="btn-save">
              💾 Save Changes
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: 'MailboxManagement',
  props: {
    mailboxes: {
      type: Array,
      default: () => []
    },
    domains: {
      type: Array,
      default: () => []
    }
  },
  data() {
    return {
      newMailbox: {
        domain_id: '',
        username: '',
        password: '',
        name: ''
      },
      showEditModal: false,
      editingMailbox: null,
      editForm: {
        name: '',
        quota: 10737418240,
        enabled: true,
        forward_to: '',
        keep_copy: true,
        auto_reply: false,
        auto_reply_msg: '',
        password: ''
      }
    };
  },
  methods: {
    handleAdd() {
      this.$emit('add', { ...this.newMailbox });
      this.newMailbox = {
        domain_id: '',
        username: '',
        password: '',
        name: ''
      };
    },
    
    openEditModal(mailbox) {
      this.editingMailbox = mailbox;
      this.editForm = {
        name: mailbox.name || '',
        quota: mailbox.quota || 10737418240,
        enabled: mailbox.enabled,
        forward_to: mailbox.forward_to || '',
        keep_copy: mailbox.keep_copy !== undefined ? mailbox.keep_copy : true,
        auto_reply: mailbox.auto_reply || false,
        auto_reply_msg: mailbox.auto_reply_msg || '',
        password: ''
      };
      this.showEditModal = true;
    },
    
    closeEditModal() {
      this.showEditModal = false;
      this.editingMailbox = null;
      this.editForm = {
        name: '',
        quota: 10737418240,
        enabled: true,
        forward_to: '',
        keep_copy: true,
        auto_reply: false,
        auto_reply_msg: '',
        password: ''
      };
    },
    
    handleEdit() {
      const payload = { ...this.editForm };
      // Remove password if empty
      if (!payload.password) {
        delete payload.password;
      }
      
      this.$emit('edit', {
        id: this.editingMailbox.ID,
        ...payload
      });
      this.closeEditModal();
    },
    
    getInitials(name) {
      return name
        .split(' ')
        .map(n => n[0])
        .join('')
        .toUpperCase()
        .substring(0, 2);
    },
    
    formatQuota(used, total) {
      if (!used) return '0 MB';
      if (!total) return `${used} MB / ∞`;
      const percent = ((used / total) * 100).toFixed(0);
      return `${used} / ${total} MB (${percent}%)`;
    },
    
    formatDate(date) {
      if (!date) return 'Never';
      return new Date(date).toLocaleDateString();
    },
    
    openWebmail(mailbox) {
      this.$emit('open-webmail', mailbox);
    }
  }
};
</script>

<style scoped>
.mailbox-management {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.add-form-card, .mailboxes-card {
  background: white;
  border-radius: 12px;
  padding: 24px;
  box-shadow: 0 2px 8px rgba(0,0,0,0.1);
}

h2 {
  margin: 0 0 20px 0;
  font-size: 1.3rem;
  color: #333;
}

.add-form {
  display: flex;
  flex-direction: column;
  gap: 15px;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
}

.form-input, .form-select {
  padding: 12px;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  font-size: 15px;
  transition: border-color 0.2s;
}

.form-input:focus, .form-select:focus {
  outline: none;
  border-color: #3b82f6;
}

.btn-add {
  padding: 12px 24px;
  background: #10b981;
  color: white;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-weight: 600;
  transition: background 0.2s;
  align-self: flex-start;
}

.btn-add:hover {
  background: #059669;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.btn-icon {
  padding: 8px 12px;
  background: #f3f4f6;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 16px;
  transition: background 0.2s;
}

.btn-icon:hover {
  background: #e5e7eb;
}

.empty-state {
  text-align: center;
  padding: 40px;
  color: #666;
  font-size: 16px;
}

.mailboxes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 20px;
}

.mailbox-card {
  border: 2px solid #e0e0e0;
  border-radius: 12px;
  padding: 20px;
  transition: all 0.2s;
}

.mailbox-card:hover {
  border-color: #3b82f6;
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.1);
}

.mailbox-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.mailbox-avatar {
  width: 50px;
  height: 50px;
  border-radius: 50%;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 18px;
}

.mailbox-info {
  flex: 1;
}

.mailbox-info h3 {
  margin: 0;
  font-size: 1.1rem;
  color: #333;
}

.mailbox-email {
  margin: 4px 0 0 0;
  color: #666;
  font-size: 14px;
}

.status-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.status-dot.enabled {
  background: #10b981;
}

.status-dot.disabled {
  background: #ef4444;
}

.mailbox-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  margin-bottom: 16px;
  padding: 12px;
  background: #f9fafb;
  border-radius: 8px;
}

.stat-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.stat-label {
  font-size: 12px;
  color: #666;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}

.stat-value {
  font-size: 14px;
  color: #333;
  font-weight: 600;
}

.mailbox-actions {
  display: flex;
  gap: 8px;
}

.btn-action {
  flex: 1;
  padding: 10px;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-weight: 600;
  font-size: 14px;
  transition: all 0.2s;
}

.btn-webmail {
  background: #3b82f6;
  color: white;
}

.btn-webmail:hover {
  background: #2563eb;
}

.btn-settings {
  background: #f3f4f6;
  color: #333;
}

.btn-settings:hover {
  background: #e5e7eb;
}

.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal-content {
  background: white;
  border-radius: 12px;
  max-width: 600px;
  width: 100%;
  max-height: 90vh;
  overflow-y: auto;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.2);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  border-bottom: 1px solid #e0e0e0;
  position: sticky;
  top: 0;
  background: white;
  border-radius: 12px 12px 0 0;
}

.modal-header h3 {
  margin: 0;
  font-size: 1.2rem;
  color: #333;
}

.btn-close {
  background: none;
  border: none;
  font-size: 24px;
  cursor: pointer;
  color: #666;
  padding: 0;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  transition: background 0.2s;
}

.btn-close:hover {
  background: #f3f4f6;
}

.edit-form {
  padding: 24px;
}

.form-group {
  margin-bottom: 20px;
}

.form-group label {
  display: block;
  margin-bottom: 8px;
  font-weight: 600;
  color: #333;
  font-size: 14px;
}

.form-textarea {
  width: 100%;
  padding: 12px;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  font-size: 15px;
  font-family: inherit;
  transition: border-color 0.2s;
  resize: vertical;
}

.form-textarea:focus {
  outline: none;
  border-color: #3b82f6;
}

.form-hint {
  display: block;
  margin-top: 4px;
  font-size: 12px;
  color: #666;
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  font-weight: 500;
}

.form-checkbox {
  width: 18px;
  height: 18px;
  cursor: pointer;
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding-top: 20px;
  margin-top: 20px;
  border-top: 1px solid #e0e0e0;
}

.btn-cancel {
  padding: 10px 20px;
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

.btn-save {
  padding: 10px 20px;
  background: #10b981;
  color: white;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-weight: 600;
  transition: background 0.2s;
}

.btn-save:hover {
  background: #059669;
}
</style>
