// Main navigation tabs. The active tab is derived from the signed-in user on
// every render, never stored on its own: a selection only applies to the
// account (id and role) that made it, and the Dashboard tab only exists for
// admins. This is display logic only; the backend authorizes every
// dashboard request (a user gets 403).

export type MainTab = 'dashboard' | 'products';

export interface TabUser {
  id: number;
  role: string;
}

export interface TabSelection {
  // tabOwner() of the user who picked the tab.
  owner: string;
  tab: MainTab;
}

export const MAIN_TAB_LABELS: Readonly<Record<MainTab, string>> = {
  dashboard: 'Dashboard',
  products: 'Sản phẩm',
};

const ADMIN_ROLE = 'admin';

export function isAdminUser(user: TabUser | null | undefined): boolean {
  return user != null && user.role === ADMIN_ROLE;
}

/** Tabs the user may see: Dashboard and Sản phẩm for an admin, Sản phẩm otherwise. */
export function visibleTabs(user: TabUser | null | undefined): MainTab[] {
  if (!user) {
    return [];
  }
  return isAdminUser(user) ? ['dashboard', 'products'] : ['products'];
}

/** Dashboard for an admin, Sản phẩm for anyone else, null when signed out. */
export function defaultTab(user: TabUser | null | undefined): MainTab | null {
  if (!user) {
    return null;
  }
  return isAdminUser(user) ? 'dashboard' : 'products';
}

/** Identifies the account a selection belongs to; a role change is a new owner. */
export function tabOwner(user: TabUser): string {
  return `${user.id}:${user.role}`;
}

/**
 * Returns the tab to show: the selection if it was made by this very account
 * and is one of its visible tabs, the default tab otherwise. Null when
 * signed out.
 */
export function resolveActiveTab(
  user: TabUser | null | undefined,
  selection: TabSelection | null | undefined
): MainTab | null {
  if (!user) {
    return null;
  }
  if (selection && selection.owner === tabOwner(user) && visibleTabs(user).includes(selection.tab)) {
    return selection.tab;
  }
  return defaultTab(user);
}
