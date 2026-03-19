import type { IconType } from 'react-icons';
import {
  RiFolderOpenLine,
  RiHome5Line,
  RiPulseLine,
  RiRobot2Line,
  RiSettings3Line,
} from 'react-icons/ri';

export type NavItem = {
  to: string;
  label: string;
  summary: string;
  icon: IconType;
};

export const navItems: NavItem[] = [
  {
    to: '/home',
    label: 'Home',
    summary: 'Talk to the base agent.',
    icon: RiHome5Line,
  },
  {
    to: '/projects',
    label: 'Projects',
    summary: 'Organize work after it starts.',
    icon: RiFolderOpenLine,
  },
  {
    to: '/agents',
    label: 'Agents',
    summary: 'See which execution surfaces are available.',
    icon: RiRobot2Line,
  },
  {
    to: '/activity',
    label: 'Activity',
    summary: 'Inspect runs, logs, and approvals only when needed.',
    icon: RiPulseLine,
  },
  {
    to: '/settings',
    label: 'Settings',
    summary: 'Connect providers, channels, and defaults.',
    icon: RiSettings3Line,
  },
];
