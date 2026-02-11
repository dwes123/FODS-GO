"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";

interface Player {
    id: string;
    first_name: string;
    last_name: string;
    position: string;
    mlb_team: string;
    status: string;
}

interface TeamDetail {
    id: string;
    name: string;
    owner: string;
    players: Player[];
}

export default function TeamRoster() {
    const params = useParams(); // This grabs the ID from the URL
    const [team, setTeam] = useState<TeamDetail | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        // Fetch the specific team
        // Note: params.id is the ID of the team you clicked
        fetch(`http://localhost:8080/teams/${params.id}`)
            .then((res) => res.json())
            .then((data) => {
                setTeam(data);
                setLoading(false);
            })
            .catch((err) => {
                console.error(err);
                setLoading(false);
            });
    }, [params.id]);

    if (loading) return <div className="p-10 text-xl font-mono animate-pulse">Scouting Roster...</div>;
    if (!team) return <div className="p-10 text-red-600">Team not found.</div>;

    return (
        <div className="min-h-screen bg-slate-50 p-8">
            <div className="max-w-5xl mx-auto">

                {/* Breadcrumb / Back Button */}
                <Link href="/" className="text-sm text-slate-500 hover:text-blue-600 mb-6 inline-block">
                    &larr; Back to Dashboard
                </Link>

                {/* Team Header */}
                <div className="bg-white rounded-xl shadow-sm border border-slate-200 p-8 mb-8">
                    <h1 className="text-4xl font-extrabold text-slate-900">{team.name}</h1>
                    <p className="text-lg text-slate-500 mt-2">Owner: <span className="font-medium text-slate-800">{team.owner}</span></p>
                </div>

                {/* Roster Table */}
                <div className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden">
                    <div className="bg-slate-900 px-6 py-4 border-b border-slate-800">
                        <h2 className="text-lg font-bold text-white">Active Roster</h2>
                    </div>

                    <table className="w-full text-left">
                        <thead className="bg-slate-50 border-b border-slate-200">
                        <tr>
                            <th className="px-6 py-3 text-xs font-semibold text-slate-500 uppercase">Pos</th>
                            <th className="px-6 py-3 text-xs font-semibold text-slate-500 uppercase">Player</th>
                            <th className="px-6 py-3 text-xs font-semibold text-slate-500 uppercase">MLB Team</th>
                            <th className="px-6 py-3 text-xs font-semibold text-slate-500 uppercase">Status</th>
                        </tr>
                        </thead>
                        <tbody className="divide-y divide-slate-100">
                        {team.players && team.players.length > 0 ? (
                            team.players.map((player) => (
                                <tr key={player.id} className="hover:bg-slate-50 transition">
                                    <td className="px-6 py-4 font-mono text-slate-600 font-bold">{player.position}</td>
                                    <td className="px-6 py-4 font-medium text-slate-900">{player.first_name} {player.last_name}</td>
                                    <td className="px-6 py-4 text-slate-500">{player.mlb_team}</td>
                                    <td className="px-6 py-4">
                      <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-100 text-green-800">
                        {player.status}
                      </span>
                                    </td>
                                </tr>
                            ))
                        ) : (
                            <tr>
                                <td colSpan={4} className="px-6 py-8 text-center text-slate-400 italic">
                                    No players on this roster.
                                </td>
                            </tr>
                        )}
                        </tbody>
                    </table>
                </div>

            </div>
        </div>
    );
}