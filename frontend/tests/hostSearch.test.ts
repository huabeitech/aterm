import test from 'node:test'
import assert from 'node:assert/strict'
import {searchHosts} from '../src/renderer/src/components/hosts/hostSearch'
import {fixtureHost as host} from './support/fixtures'

test('host search includes alias, hostname and remark but excludes connection secrets and paths', () => {
  const hosts=[host('production',{hostName:'gateway.test',remark:'customer cluster',user:'hidden-user',identityFile:'/hidden/key',proxyJump:'hidden-proxy'})]
  for (const query of ['production','gateway','customer']) assert.equal(searchHosts(hosts,query).length,1)
  for (const query of ['hidden-user','hidden/key','hidden-proxy']) assert.equal(searchHosts(hosts,query).length,0)
})
test('host search ranks exact aliases before endpoint and remark matches', () => {
  const hosts=[host('remark',{remark:'production'}),host('endpoint',{hostName:'production'}),host('production')]
  assert.deepEqual(searchHosts(hosts,'production').map(result=>result.host.alias),['production','endpoint','remark'])
})
test('host search supports compact fuzzy queries and reports matching positions', () => {
  const result=searchHosts([host('production-api')],'prdapi')[0]
  assert.equal(result.host.alias,'production-api')
  assert.ok(result.matches.alias.length>0)
})
test('empty host searches deduplicate by ID and prioritize recent use', () => {
  const old=host('old',{lastUsedAt:'2024-01-01'}), recent=host('recent',{lastUsedAt:'2026-01-01'})
  assert.deepEqual(searchHosts([old,recent,old],'').map(result=>result.host.id),['recent','old'])
})
