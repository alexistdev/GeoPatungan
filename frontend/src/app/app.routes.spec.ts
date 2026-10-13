import { adminGuard } from './core/guards';
import { routes } from './app.routes';

const flatten = (rs: typeof routes): typeof routes => rs.flatMap((r) => [r, ...flatten(r.children ?? [])]);

describe('rute', () => {
  it('membuat grup hanya untuk admin', () => {
    const r = flatten(routes).find((x) => x.path === 'grup/baru');
    expect(r).toBeDefined();
    expect(r?.canActivate ?? []).toContain(adminGuard);
  });
});
