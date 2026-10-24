// Entry point of the public room-join page (/redock/join.html on the SFU).
// It is a separate, login-free app: participants authenticate with the room
// token in the URL fragment, never with a Redock account.
import { createApp } from 'vue'
import JoinRoom from './JoinRoom.vue'
import '../css/main.css'

createApp(JoinRoom).mount('#app')
