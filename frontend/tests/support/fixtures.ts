import type {domain} from '../../src/shared/contracts/domain'

export const fixtureHost = (alias: string, values: Partial<domain.Host> = {}): domain.Host => ({
  id:alias, name:alias, alias, hostName:'server.test', user:'', directoryID:'', port:22,
  identityFile:'', passwordAuth:false, proxyJump:'', jumpHostID:'', forwardAgent:false,
  compression:false, serverAliveInterval:0, serverAliveCountMax:0, connectTimeout:0,
  remark:'', lastUsedAt:'', createdAt:'', updatedAt:'', sortOrders:{}, ...values
})
