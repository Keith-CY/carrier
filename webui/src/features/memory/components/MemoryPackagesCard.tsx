import type { MemoryData } from '../useMemoryData';
import { renderMemorySection } from './shared';

export function MemoryPackagesCard({ data }: { data: MemoryData }) {
  const { payload } = data;

  return (
    <div className="card dashboard-stack">
      <div className="section-head">
        <div>
          <h3>Packages</h3>
          <p className="text-dim">Current packages, attachments, and grants for the selected subject.</p>
        </div>
      </div>
      <div id="memory-entry-list">
        <div className="memory-section">
          <h4>Entries</h4>
          {renderMemorySection(payload.entries.map((entry: any) => ({
            title: String(entry?.id || 'unknown'),
            meta: [`type: ${String(entry?.type || 'unknown')}`],
          })), 'No memory packages found.')}
        </div>
        <div className="memory-section">
          <h4>Attachments</h4>
          {renderMemorySection(payload.attachments.map((attachment: any) => ({
            title: String(attachment?.memory_id || 'unknown'),
            meta: [`agent: ${String(attachment?.agent_id || 'unknown')}`],
          })), 'No attachments for this subject.')}
        </div>
        <div className="memory-section">
          <h4>Grants</h4>
          {renderMemorySection(payload.grants.map((grant: any) => ({
            title: String(grant?.scope || 'unknown'),
            meta: [`subject: ${String(grant?.subject || 'unknown')}`],
          })), 'No grants for this subject.')}
        </div>
      </div>
    </div>
  );
}
