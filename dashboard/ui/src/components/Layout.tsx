import { NavLink, Outlet } from 'react-router-dom';
import './Layout.css';

const NAV_GROUPS = [
  {
    title: 'Monitor',
    items: [
      { label: 'Overview', to: '/' },
      { label: 'Live', to: '/live' },
    ],
  },
  {
    title: 'Experiments',
    items: [
      { label: 'All Experiments', to: '/experiments' },
      { label: 'Suites', to: '/suites' },
    ],
  },
  {
    title: 'Insights',
    items: [
      { label: 'Operators', to: '/operators' },
      { label: 'Knowledge', to: '/knowledge' },
    ],
  },
];

export function Layout() {
  return (
    <div className="app-layout">
      <nav className="app-sidebar">
        <div className="app-logo">
          Operator <span>Chaos</span>
        </div>
        {NAV_GROUPS.map((group) => (
          <div key={group.title}>
            <div className="nav-section-title">{group.title}</div>
            {group.items.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.to === '/'}
                className={({ isActive }) => `nav-item${isActive ? ' active' : ''}`}
              >
                {item.label}
              </NavLink>
            ))}
          </div>
        ))}
      </nav>
      <main className="app-content">
        <Outlet />
      </main>
    </div>
  );
}
