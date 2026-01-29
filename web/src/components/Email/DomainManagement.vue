<template>
  <div class="domain-management">
    <!-- Add Domain Form -->
    <div class="add-form-card">
      <h2>➕ Add New Domain</h2>
      <form @submit.prevent="handleAdd" class="add-form">
        <div class="form-row">
          <input
            v-model="newDomain.domain"
            type="text"
            placeholder="example.com"
            required
            class="form-input"
          />
          <input
            v-model="newDomain.description"
            type="text"
            placeholder="Description (optional)"
            class="form-input"
          />
          <button type="submit" class="btn-add">Add Domain</button>
        </div>
      </form>
    </div>

    <!-- Domains List -->
    <div class="domains-card">
      <div class="card-header">
        <h2>🌐 Email Domains ({{ domains.length }})</h2>
        <button @click="$emit('refresh')" class="btn-icon">🔄</button>
      </div>

      <div v-if="domains.length === 0" class="empty-state">
        <p>📭 No domains yet. Add your first domain above!</p>
      </div>

      <div v-else class="domains-list">
        <div v-for="domain in domains" :key="domain.ID" class="domain-item">
          <div class="domain-header">
            <div class="domain-name">
              <h3>{{ domain.domain }}</h3>
              <span v-if="domain.description" class="domain-desc">{{ domain.description }}</span>
            </div>
            <div class="domain-actions">
              <span :class="['status-badge', domain.enabled ? 'enabled' : 'disabled']">
                {{ domain.enabled ? '✅ Active' : '❌ Disabled' }}
              </span>
              <button @click="openEditModal(domain)" class="btn-edit" title="Edit Domain">
                ✏️
              </button>
            </div>
          </div>

          <div class="domain-info">
            <div class="info-row">
              <span class="info-label">Mailboxes:</span>
              <span class="info-value">{{ getMailboxCount(domain.ID) }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">DNS Configured:</span>
              <span class="info-value">{{ domain.dns_configured ? '✅ Yes' : '❌ No' }}</span>
            </div>
            <div class="info-row">
              <span class="info-label">DKIM:</span>
              <span class="info-value">{{ domain.dkim_selector || 'mail' }}</span>
            </div>
          </div>

          <div class="domain-dns">
            <button @click="domain.showDNS = !domain.showDNS" class="btn-dns">
              {{ domain.showDNS ? '▼' : '▶' }} DNS Records
            </button>
            
            <div v-if="domain.showDNS" class="dns-records">
              <div class="dns-record">
                <span class="dns-type">MX</span>
                <code>{{ domain.mx_record || 'mail.' + domain.domain }}</code>
              </div>
              <div class="dns-record">
                <span class="dns-type">SPF</span>
                <code>{{ domain.spf_record }}</code>
              </div>
              <div class="dns-record">
                <span class="dns-type">DKIM</span>
                <code>{{ domain.dkim_public_key ? domain.dkim_public_key.substring(0, 50) + '...' : 'Not generated' }}</code>
              </div>
              <div class="dns-record">
                <span class="dns-type">DMARC</span>
                <code>{{ domain.dmarc_record }}</code>
              </div>
              
              <button class="btn-cloudflare" title="Auto-configure DNS via Cloudflare">
                ☁️ Configure DNS (Cloudflare)
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Edit Domain Modal -->
    <div v-if="showEditModal" class="modal-overlay" @click="closeEditModal">
      <div class="modal-content" @click.stop>
        <div class="modal-header">
          <h3>✏️ Edit Domain: {{ editingDomain.domain }}</h3>
          <button @click="closeEditModal" class="btn-close">✕</button>
        </div>
        
        <form @submit.prevent="handleEdit" class="edit-form">
          <div class="form-group">
            <label>Description</label>
            <input
              v-model="editForm.description"
              type="text"
              placeholder="Domain description"
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
              <span>Enable Domain</span>
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
  name: 'DomainManagement',
  props: {
    domains: {
      type: Array,
      default: () => []
    }
  },
  data() {
    return {
      newDomain: {
        domain: '',
        description: ''
      },
      showEditModal: false,
      editingDomain: null,
      editForm: {
        description: '',
        enabled: true
      }
    };
  },
  methods: {
    handleAdd() {
      this.$emit('add', { ...this.newDomain });
      this.newDomain = { domain: '', description: '' };
    },
    
    openEditModal(domain) {
      this.editingDomain = domain;
      this.editForm = {
        description: domain.description || '',
        enabled: domain.enabled
      };
      this.showEditModal = true;
    },
    
    closeEditModal() {
      this.showEditModal = false;
      this.editingDomain = null;
      this.editForm = {
        description: '',
        enabled: true
      };
    },
    
    handleEdit() {
      this.$emit('edit', {
        id: this.editingDomain.ID,
        ...this.editForm
      });
      this.closeEditModal();
    },
    
    getMailboxCount(domainId) {
      // This would be populated from parent or API
      return 0;
    }
  }
};
</script>

<style scoped>
.domain-management {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.add-form-card, .domains-card {
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

.form-row {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.form-input {
  flex: 1;
  min-width: 200px;
  padding: 12px;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  font-size: 15px;
  transition: border-color 0.2s;
}

.form-input:focus {
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

.domains-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.domain-item {
  border: 2px solid #e0e0e0;
  border-radius: 12px;
  padding: 20px;
  transition: border-color 0.2s;
}

.domain-item:hover {
  border-color: #3b82f6;
}

.domain-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 16px;
}

.domain-name h3 {
  margin: 0;
  font-size: 1.4rem;
  color: #333;
}

.domain-desc {
  color: #666;
  font-size: 14px;
}

.status-badge {
  padding: 6px 12px;
  border-radius: 16px;
  font-size: 13px;
  font-weight: 600;
}

.status-badge.enabled {
  background: #d1fae5;
  color: #065f46;
}

.status-badge.disabled {
  background: #fee2e2;
  color: #991b1b;
}

.domain-info {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.info-row {
  display: flex;
  justify-content: space-between;
  padding: 8px 12px;
  background: #f9fafb;
  border-radius: 6px;
}

.info-label {
  color: #666;
  font-size: 14px;
}

.info-value {
  color: #333;
  font-weight: 600;
  font-size: 14px;
}

.domain-dns {
  border-top: 1px solid #e0e0e0;
  padding-top: 16px;
}

.btn-dns {
  padding: 8px 16px;
  background: #f3f4f6;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-weight: 500;
  transition: background 0.2s;
}

.btn-dns:hover {
  background: #e5e7eb;
}

.dns-records {
  margin-top: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.dns-record {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px;
  background: #f9fafb;
  border-radius: 6px;
}

.dns-type {
  padding: 4px 8px;
  background: #3b82f6;
  color: white;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 700;
  min-width: 60px;
  text-align: center;
}

.dns-record code {
  flex: 1;
  font-family: 'Courier New', monospace;
  font-size: 13px;
  color: #333;
  overflow-x: auto;
}

.btn-cloudflare {
  margin-top: 12px;
  padding: 10px 20px;
  background: #f59e0b;
  color: white;
  border: none;
  border-radius: 8px;
  cursor: pointer;
  font-weight: 600;
  transition: background 0.2s;
}

.btn-cloudflare:hover {
  background: #d97706;
}

.domain-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.btn-edit {
  padding: 8px 12px;
  background: #3b82f6;
  color: white;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  transition: background 0.2s;
}

.btn-edit:hover {
  background: #2563eb;
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
  max-width: 500px;
  width: 100%;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.2);
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  border-bottom: 1px solid #e0e0e0;
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
