"use client";

import { useEffect, useState } from "react";
import Link from "next/link"; // <--- Added this!

interface Team {
    id: string;
    name: string;
}

interface League {
    id: string;
    name: string;
    teams: Team[];
}

export default function Dashboard() {
    const [leagues, setLeagues] = useState<League[]>([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        fetch("http://localhost:8080/dashboard")
            .then((res) => res.json())
            .then((data) => {
                setLeagues(data.leagues || []);
                setLoading(false);
            })
            .catch((err) => {
                console.error("Failed to load dashboard:", err);
                setLoading(false);
            });
    }, []);

    if (loading) return <div className="p-10 text-xl font-mono animate-pulse">Loading Commissioner Data...</div>;

    return (
        <div className="min-h-screen bg-slate-50 p-8">
            <div className="max-w-7xl mx-auto">
                <header className="mb-10 flex items-center justify-between">
                    <div>
                        <h1 className="text-4xl font-extrabold text-slate-900 tracking-tight">⚾️ Moneyball Dynasty</h1>
                        <p className="text-slate-500 mt-2 text-lg">League Operations Center</p>
                    </div>
                </header>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                    {leagues.map((league) => (
                        <div key={league.id} className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden hover:shadow-md transition">
                            <div className="bg-slate-900 px-6 py-4 border-b border-slate-800">
                                <h2 className="text-xl font-bold text-white flex justify-between items-center">
                                    {league.name}
                                    <span className="text-xs bg-slate-700 px-2 py-1 rounded text-slate-300 font-mono">ID: {league.id.slice(0,4)}...</span>
                                </h2>
                            </div>
                            <div className="p-6">
                                <h3 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-4">Teams</h3>
                                {league.teams && league.teams.length > 0 ? (
                                    <ul className="space-y-3">
                                        {league.teams.map((team) => (
                                            <li key={team.id} className="flex items-center justify-between p-3 bg-slate-50 rounded-lg border border-slate-100">
                                                <span className="font-medium text-slate-800">{team.name}</span>

                                                {/* THIS IS THE FIX: We use a Link component now */}
                                                <Link
                                                    href={`/teams/${team.id}`}
                                                    className="text-xs text-blue-600 font-semibold hover:underline"
                                                >
                                                    Manage &rarr;
                                                </Link>

                                            </li>
                                        ))}
                                    </ul>
                                ) : (
                                    <p className="text-sm text-slate-400 italic">No teams in this league yet.</p>
                                )}
                            </div>
                        </div>
                    ))}
                </div>
            </div>
        </div>
    );
}