import React from 'react';
import {Composition} from 'remotion';
import {FullFilm} from './FullFilm';
export const Root=()=> <Composition id="RoomToBuild" component={FullFilm} width={1920} height={1080} fps={30} durationInFrames={2700} defaultProps={{bgm:true,sound:true}}/>;
