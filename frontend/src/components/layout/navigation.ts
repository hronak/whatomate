import {
  LayoutDashboard,
  MessageSquare,
  Bot,
  FileText,
  Megaphone,
  Settings,
  Users,
  Contact,
  Workflow,
  Sparkles,
  Key,
  UserX,
  MessageSquareText,
  Webhook,
  BarChart3,
  ShieldCheck,
  Zap,
  Shield,
  LineChart,
  Tags,
  PhoneCall,
  PhoneForwarded,
  ScrollText
} from '@lucide/vue'
import type { Component } from 'vue'

export interface NavItem {
  name: string
  path: string
  icon: Component
  permission?: string
  childPermissions?: string[]
  children?: NavItem[]
  /**
   * i18n key for the heading a child is filed under in the sub-nav column.
   * Only read for children rendered in the second column (see AppLayout);
   * children without one fall into an unlabelled leading group.
   */
  group?: string
}

export interface NavSection {
  label: string
  items: NavItem[]
  /** Permissions needed to show section — at least one must pass */
  permissions: string[]
  /** Pin to bottom of sidebar */
  pinBottom?: boolean
}

export const navigationSections: NavSection[] = [
  {
    label: 'nav.sectionMain',
    permissions: ['analytics', 'chat'],
    items: [
      {
        name: 'nav.dashboard',
        path: '/',
        icon: LayoutDashboard,
        permission: 'analytics'
      },
      {
        name: 'nav.chat',
        path: '/chat',
        icon: MessageSquare,
        permission: 'chat'
      },
    ]
  },
  {
    label: 'nav.sectionMessaging',
    permissions: ['settings.chatbot', 'chatbot.keywords', 'flows.chatbot', 'chatbot.ai', 'transfers', 'campaigns', 'templates', 'flows.whatsapp'],
    items: [
      {
        name: 'nav.chatbot',
        path: '/chatbot',
        icon: Bot,
        permission: 'settings.chatbot',
        childPermissions: ['settings.chatbot', 'chatbot.keywords', 'flows.chatbot', 'chatbot.ai', 'transfers'],
        children: [
          { name: 'nav.overview', path: '/chatbot', icon: Bot, permission: 'settings.chatbot' },
          { name: 'nav.keywords', path: '/chatbot/keywords', icon: Key, permission: 'chatbot.keywords' },
          { name: 'nav.flows', path: '/chatbot/flows', icon: Workflow, permission: 'flows.chatbot' },
          { name: 'nav.aiContexts', path: '/chatbot/ai', icon: Sparkles, permission: 'chatbot.ai' },
          { name: 'nav.transfers', path: '/chatbot/transfers', icon: UserX, permission: 'transfers' }
        ]
      },
      {
        name: 'nav.campaigns',
        path: '/campaigns',
        icon: Megaphone,
        permission: 'campaigns'
      },
      {
        name: 'nav.templates',
        path: '/templates',
        icon: FileText,
        permission: 'templates'
      },
      {
        name: 'nav.flows',
        path: '/flows',
        icon: Workflow,
        permission: 'flows.whatsapp'
      },
    ]
  },
  {
    label: 'nav.sectionCalling',
    permissions: ['call_logs', 'ivr_flows', 'call_transfers'],
    items: [
      { name: 'nav.callLogs', path: '/calling/logs', icon: PhoneCall, permission: 'call_logs' },
      { name: 'nav.ivrFlows', path: '/calling/ivr-flows', icon: Workflow, permission: 'ivr_flows' },
      { name: 'nav.callTransfers', path: '/calling/transfers', icon: PhoneForwarded, permission: 'call_transfers' },
    ]
  },
  {
    label: 'nav.sectionAnalytics',
    permissions: ['analytics.agents', 'analytics'],
    items: [
      {
        name: 'nav.agentAnalytics',
        path: '/analytics/agents',
        icon: BarChart3,
        permission: 'analytics.agents'
      },
      {
        name: 'nav.metaInsights',
        path: '/analytics/meta-insights',
        icon: LineChart,
        permission: 'analytics'
      },
    ]
  },
  {
    label: '',
    permissions: ['settings.general', 'settings.chatbot', 'accounts', 'contacts', 'canned_responses', 'tags', 'teams', 'users', 'roles', 'api_keys', 'webhooks', 'custom_actions', 'settings.sso', 'audit_logs'],
    pinBottom: true,
    items: [
      {
        name: 'nav.settings',
        path: '/settings',
        icon: Settings,
        permission: 'settings.general',
        childPermissions: ['settings.general', 'settings.chatbot', 'accounts', 'contacts', 'canned_responses', 'tags', 'teams', 'users', 'roles', 'api_keys', 'webhooks', 'custom_actions', 'settings.sso', 'audit_logs'],
        children: [
          { name: 'nav.general', path: '/settings', icon: Settings, permission: 'settings.general', group: 'nav.groupWorkspace' },
          { name: 'nav.chatbot', path: '/settings/chatbot', icon: Bot, permission: 'settings.chatbot', group: 'nav.groupWorkspace' },
          { name: 'nav.accounts', path: '/settings/accounts', icon: Users, permission: 'accounts', group: 'nav.groupWorkspace' },
          { name: 'nav.contacts', path: '/settings/contacts', icon: Contact, permission: 'contacts', group: 'nav.groupInbox' },
          { name: 'nav.cannedResponses', path: '/settings/canned-responses', icon: MessageSquareText, permission: 'canned_responses', group: 'nav.groupInbox' },
          { name: 'nav.tags', path: '/settings/tags', icon: Tags, permission: 'tags', group: 'nav.groupInbox' },
          { name: 'nav.teams', path: '/settings/teams', icon: Users, permission: 'teams', group: 'nav.groupPeople' },
          { name: 'nav.users', path: '/settings/users', icon: Users, permission: 'users', group: 'nav.groupPeople' },
          { name: 'nav.roles', path: '/settings/roles', icon: Shield, permission: 'roles', group: 'nav.groupPeople' },
          { name: 'nav.sso', path: '/settings/sso', icon: ShieldCheck, permission: 'settings.sso', group: 'nav.groupPeople' },
          { name: 'nav.apiKeys', path: '/settings/api-keys', icon: Key, permission: 'api_keys', group: 'nav.groupDeveloper' },
          { name: 'nav.webhooks', path: '/settings/webhooks', icon: Webhook, permission: 'webhooks', group: 'nav.groupDeveloper' },
          { name: 'nav.customActions', path: '/settings/custom-actions', icon: Zap, permission: 'custom_actions', group: 'nav.groupDeveloper' },
          { name: 'nav.auditLogs', path: '/settings/audit-logs', icon: ScrollText, permission: 'audit_logs', group: 'nav.groupSystem' }
        ]
      }
    ]
  }
]

// Flat list for backward compatibility (used by AppLayout computed)
export const navigationItems: NavItem[] = navigationSections.flatMap(s => s.items)
