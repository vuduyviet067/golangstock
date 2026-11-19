import { describe, it } from 'node:test';
import assert from 'node:assert/strict';
import {
  MAIN_TAB_LABELS,
  defaultTab,
  isAdminUser,
  resolveActiveTab,
  tabOwner,
  visibleTabs,
  type MainTab,
  type TabSelection,
} from '../src/utils/dashboardTabs';

const admin = { id: 1, role: 'admin' };
const user = { id: 2, role: 'user' };
const selectionBy = (who: { id: number; role: string }, tab: MainTab): TabSelection => ({ owner: tabOwner(who), tab });

describe('visibleTabs', () => {
  it('shows Dashboard and Sản phẩm to an admin', () => {
    assert.deepEqual(visibleTabs(admin), ['dashboard', 'products']);
  });

  it('shows only Sản phẩm to a user', () => {
    assert.deepEqual(visibleTabs(user), ['products']);
  });

  it('shows only Sản phẩm for any other role, including a differently cased admin', () => {
    assert.deepEqual(visibleTabs({ id: 3, role: 'manager' }), ['products']);
    assert.deepEqual(visibleTabs({ id: 3, role: 'Admin' }), ['products']);
    assert.deepEqual(visibleTabs({ id: 3, role: '' }), ['products']);
  });

  it('shows nothing when signed out', () => {
    assert.deepEqual(visibleTabs(null), []);
    assert.deepEqual(visibleTabs(undefined), []);
  });
});

describe('defaultTab', () => {
  it('is Dashboard for an admin and Sản phẩm for a user', () => {
    assert.equal(defaultTab(admin), 'dashboard');
    assert.equal(defaultTab(user), 'products');
    assert.equal(defaultTab({ id: 3, role: 'manager' }), 'products');
  });

  it('is null when signed out', () => {
    assert.equal(defaultTab(null), null);
  });
});

describe('isAdminUser and tabOwner', () => {
  it('only the exact admin role is an admin', () => {
    assert.equal(isAdminUser(admin), true);
    assert.equal(isAdminUser(user), false);
    assert.equal(isAdminUser({ id: 1, role: 'ADMIN' }), false);
    assert.equal(isAdminUser(null), false);
  });

  it('the owner changes with the id and with the role', () => {
    assert.equal(tabOwner(admin), '1:admin');
    assert.notEqual(tabOwner(admin), tabOwner({ id: 1, role: 'user' }));
    assert.notEqual(tabOwner(admin), tabOwner({ id: 9, role: 'admin' }));
  });
});

describe('resolveActiveTab', () => {
  it('starts an admin on Dashboard and a user on Sản phẩm', () => {
    assert.equal(resolveActiveTab(admin, null), 'dashboard');
    assert.equal(resolveActiveTab(user, null), 'products');
  });

  it('lets an admin switch between Dashboard and Sản phẩm', () => {
    assert.equal(resolveActiveTab(admin, selectionBy(admin, 'products')), 'products');
    assert.equal(resolveActiveTab(admin, selectionBy(admin, 'dashboard')), 'dashboard');
  });

  it('never gives a user the Dashboard, even with a forged selection', () => {
    assert.equal(resolveActiveTab(user, selectionBy(user, 'dashboard')), 'products');
    assert.equal(resolveActiveTab(user, { owner: tabOwner(user), tab: 'dashboard' }), 'products');
  });

  it('ignores the selection of the previous account after logout and login', () => {
    // An admin picked Dashboard (or Sản phẩm), logged out, and a user logged in.
    assert.equal(resolveActiveTab(user, selectionBy(admin, 'dashboard')), 'products');
    // A user logged out and an admin logged in: the admin starts on Dashboard.
    assert.equal(resolveActiveTab(admin, selectionBy(user, 'products')), 'dashboard');
    // Another admin does not inherit the first admin's choice.
    assert.equal(resolveActiveTab({ id: 7, role: 'admin' }, selectionBy(admin, 'products')), 'dashboard');
  });

  it('drops the selection when the role of the same account changes', () => {
    const demoted = { id: 1, role: 'user' };
    assert.equal(resolveActiveTab(demoted, selectionBy(admin, 'dashboard')), 'products');
    const promoted = { id: 2, role: 'admin' };
    assert.equal(resolveActiveTab(promoted, selectionBy(user, 'products')), 'dashboard');
  });

  it('ignores an unknown tab', () => {
    assert.equal(resolveActiveTab(admin, { owner: tabOwner(admin), tab: 'settings' as MainTab }), 'dashboard');
  });

  it('is null when signed out, whatever the selection', () => {
    assert.equal(resolveActiveTab(null, selectionBy(admin, 'dashboard')), null);
    assert.equal(resolveActiveTab(undefined, null), null);
  });
});

describe('MAIN_TAB_LABELS', () => {
  it('labels both tabs', () => {
    assert.deepEqual(MAIN_TAB_LABELS, { dashboard: 'Dashboard', products: 'Sản phẩm' });
  });
});
